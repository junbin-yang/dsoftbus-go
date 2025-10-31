package authmanager

import (
	"fmt"
	"net"
	"sync"
	"syscall"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// TCPServer 用于认证的TCP服务器，负责监听连接、处理数据收发及连接管理
type TCPServer struct {
	listener  net.Listener     // TCP监听器
	authMgr   *AuthManager     // 关联的认证管理器
	connMap   map[int]net.Conn // 连接映射（伪文件描述符 -> 网络连接）
	connMapMu sync.RWMutex     // 保护connMap的读写锁
	nextFd    int              // 下一个可用的伪文件描述符
	fdMu      sync.Mutex       // 保护nextFd的互斥锁
	stopChan  chan struct{}    // 用于通知停止的通道
	wg        sync.WaitGroup   // 用于等待所有goroutine结束
}

// NewTCPServer 创建新的TCP服务器实例
// 参数：
//   - authMgr：关联的认证管理器
// 返回：
//   - 初始化后的TCPServer实例
func NewTCPServer(authMgr *AuthManager) *TCPServer {
	return &TCPServer{
		authMgr:  authMgr,
		connMap:  make(map[int]net.Conn),
		nextFd:   1000, // 伪文件描述符起始值（避免与系统fd冲突）
		stopChan: make(chan struct{}),
	}
}

// Start 启动TCP服务器并在指定地址监听
// 参数：
//   - address：监听地址（格式如":8080"）
// 返回：
//   - 错误信息（启动失败时）
func (s *TCPServer) Start(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("启动监听器失败: %v", err)
	}

	s.listener = listener
	log.Infof("[AUTH] TCP服务器已在%s启动", address)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop 停止TCP服务器
// 返回：
//   - 错误信息（停止失败时）
func (s *TCPServer) Stop() error {
	close(s.stopChan)

	if s.listener != nil {
		// 关闭监听器
		s.listener.Close()
	}

	// 关闭所有连接
	s.connMapMu.Lock()
	for fd, conn := range s.connMap {
		conn.Close()
		delete(s.connMap, fd)
	}
	s.connMapMu.Unlock()

	s.wg.Wait()
	log.Info("[AUTH] TCP服务器已停止")
	return nil
}

// allocateFd 为连接分配伪文件描述符（只用于跟踪连接）
// 返回：
//   - 分配的伪文件描述符
func (s *TCPServer) allocateFd() int {
	s.fdMu.Lock()
	defer s.fdMu.Unlock()
	fd := s.nextFd
	s.nextFd++
	return fd
}

// registerConn 注册连接并分配伪文件描述符
// 参数：
//   - conn：网络连接
// 返回：
//   - 分配的伪文件描述符
func (s *TCPServer) registerConn(conn net.Conn) int {
	fd := s.allocateFd()
	s.connMapMu.Lock()
	s.connMap[fd] = conn
	s.connMapMu.Unlock()
	return fd
}

// unregisterConn 注销连接（关闭并从映射中移除）
// 参数：
//   - fd：伪文件描述符
func (s *TCPServer) unregisterConn(fd int) {
	s.connMapMu.Lock()
	if conn, ok := s.connMap[fd]; ok {
		conn.Close()
		delete(s.connMap, fd)
	}
	s.connMapMu.Unlock()
}

// getConn 通过伪文件描述符获取连接
// 参数：
//   - fd：伪文件描述符
// 返回：
//   - 对应的网络连接（net.Conn）
func (s *TCPServer) getConn(fd int) net.Conn {
	s.connMapMu.RLock()
	defer s.connMapMu.RUnlock()
	return s.connMap[fd]
}

// acceptLoop 循环接受 incoming 连接
func (s *TCPServer) acceptLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.stopChan: // 收到停止信号
			return
		default:
		}

		// 接受新连接
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopChan: // 停止信号导致的错误，直接返回
				return
			default:
				log.Infof("[AUTH] 接受连接错误: %v", err)
				continue
			}
		}

		// 启动goroutine处理新连接
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection 处理单个连接的生命周期
// 参数：
//   - netConn：网络连接
func (s *TCPServer) handleConnection(netConn net.Conn) {
	defer s.wg.Done()
	defer netConn.Close() // 退出时关闭连接

	// 获取远程地址信息
	remoteAddr := netConn.RemoteAddr().(*net.TCPAddr)
	ip := remoteAddr.IP.String()

	// 注册连接并获取伪文件描述符
	fd := s.registerConn(netConn)
	defer s.unregisterConn(fd) // 退出时注销连接

	log.Infof("[AUTH] 新连接来自 %s (fd=%d)", ip, fd)

	// 将连接添加到认证管理器
	authConn, err := s.authMgr.AddAuthConn(fd, ip)
	if err != nil {
		log.Errorf("[AUTH] 添加认证连接失败: %v\n", err)
		return
	}
	defer s.authMgr.RemoveAuthConn(fd) // 退出时从认证管理器移除

	// 注册网络连接到认证管理器
	s.authMgr.RegisterNetConn(fd, netConn)
	defer s.authMgr.UnregisterNetConn(fd) // 退出时注销

	// 处理该连接的数据事件
	s.handleDataEvents(fd, authConn, netConn)
}

// handleDataEvents 处理连接的数据接收和处理
// 参数：
//   - fd：伪文件描述符
//   - authConn：认证连接
//   - netConn：网络连接
func (s *TCPServer) handleDataEvents(fd int, authConn *AuthConn, netConn net.Conn) {
	buf := authConn.DB.Buf // 使用认证连接的缓冲区
	used := 0              // 缓冲区中已使用的字节数

	for {
		select {
		case <-s.stopChan: // 收到停止信号
			return
		default:
		}

		// 从连接读取数据
		n, err := netConn.Read(buf[used:])
		if err != nil {
			// 非EAGAIN错误视为连接关闭（EAGAIN通常是非阻塞模式下的临时无数据）
			if err != syscall.EAGAIN {
				log.Warnf("[AUTH] 连接关闭 (fd=%d): %v", fd, err)
			}
			return
		}

		if n == 0 { // 读取到0字节表示连接关闭
			return
		}

		used += n // 更新已使用字节数

		// 处理缓冲区中的数据包
		processed := s.authMgr.ProcessPackets(authConn, netConn, buf, used)
		if processed > 0 {
			// 将未处理的数据移到缓冲区头部
			used -= processed
			if used > 0 {
				copy(buf, buf[processed:processed+used])
			}
		} else if processed < 0 {
			// 处理失败，关闭连接
			log.Errorf("[AUTH] 数据包处理失败，关闭连接 (fd=%d)", fd)
			return
		}

		// 检查缓冲区是否溢出
		if used >= len(buf) {
			log.Errorf("[AUTH] 缓冲区溢出，关闭连接 (fd=%d)", fd)
			return
		}
	}
}

// GetPort 返回服务器监听的端口
// 返回：
//   - 端口号（未启动时返回-1）
func (s *TCPServer) GetPort() int {
	if s.listener == nil {
		return -1
	}
	addr := s.listener.Addr().(*net.TCPAddr)
	return addr.Port
}
