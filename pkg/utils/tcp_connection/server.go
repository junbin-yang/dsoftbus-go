package tcp_connection

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// 负责监听连接、处理数据收发及连接管理
type BaseServer struct {
	listener  net.Listener         // 连接监听器
	connMap   map[int]net.Conn     // 连接映射（伪文件描述符 -> 网络连接）
	connMapMu sync.RWMutex         // 保护connMap的读写锁
	nextFd    int                  // 下一个可用的伪文件描述符
	fdMu      sync.Mutex           // 保护nextFd的互斥锁
	stopChan  chan struct{}        // 用于通知停止的通道
	wg        sync.WaitGroup       // 用于等待所有goroutine结束
	handler   *BaseListenerHandler // 监听器的事件回调
}

func NewBaseServer() *BaseServer {
	return &BaseServer{
		connMap:  make(map[int]net.Conn),
		nextFd:   1000, // 伪文件描述符起始值（避免与系统fd冲突）
		stopChan: make(chan struct{}),
	}
}

// StartBaseListener 启动基础监听器
// 参数：
//   info：本地监听器的配置信息（包含地址、端口等）
//   listener：监听器实例，用于处理监听器的事件回调
func (s *BaseServer) StartBaseListener(opt *SocketOption, handler *BaseListenerHandler) error {
	if opt == nil || handler == nil {
		return fmt.Errorf("参数错误")
	}

	// 创建监听器
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", opt.addr, opt.port))
	if err != nil {
		fmt.Printf("创建监听器失败：%v\n", err)
		return err
	}

	s.listener = listener
	s.handler = handler
	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// StopBaseListener 启动基础监听器
func (s *BaseServer) StopBaseListener() error {
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
	return nil
}

// allocateFd 为连接分配伪文件描述符（只用于跟踪连接）
// 返回：
//   - 分配的伪文件描述符
func (s *BaseServer) allocateFd() int {
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
func (s *BaseServer) registerConn(conn net.Conn) int {
	fd := s.allocateFd()
	s.connMapMu.Lock()
	s.connMap[fd] = conn
	s.connMapMu.Unlock()
	return fd
}

// unregisterConn 注销连接（关闭并从映射中移除）
// 参数：
//   - fd：伪文件描述符
func (s *BaseServer) unregisterConn(fd int) {
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
func (s *BaseServer) getConn(fd int) net.Conn {
	s.connMapMu.RLock()
	defer s.connMapMu.RUnlock()
	return s.connMap[fd]
}

// acceptLoop 循环接受 incoming 连接
func (s *BaseServer) acceptLoop() {
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
				log.Errorf("[BASE_LISTENER] 接受连接错误: %v", err)
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
func (s *BaseServer) handleConnection(netConn net.Conn) {
	defer s.wg.Done()
	defer netConn.Close() // 退出时关闭连接

	// 获取远程地址信息
	remoteAddr := netConn.RemoteAddr().(*net.TCPAddr)
	ip := remoteAddr.IP.String()

	// 注册连接并获取伪文件描述符
	fd := s.registerConn(netConn)
	defer s.unregisterConn(fd) // 退出时注销连接

	log.Debugf("[BASE_LISTENER] 新连接来自 %s (fd=%d)", ip, fd)

	// 处理该连接的连接事件
	connectInfo := &ConnectOption{
		socketOption: &SocketOption{addr: ip, port: remoteAddr.Port},
		netConn:      &netConn,
	}
	s.handler.onConnectEvent(fd, connectInfo)

	// 处理该连接的数据事件
	s.handleDataEvents(fd, netConn)
}

// handleDataEvents 处理连接的数据接收和处理
// 参数：
//   - fd：伪文件描述符
//   - netConn：网络连接
func (s *BaseServer) handleDataEvents(fd int, netConn net.Conn) {
	buf := make([]byte, DefaultBufSize)
	used := 0 // 缓冲区中已使用的字节数

	for {
		select {
		case <-s.stopChan: // 收到停止信号
			s.handler.onDisconnectEvent(fd)
			return
		default:
		}

		// 从连接读取数据（阻塞模式，无数据时会等待）
		n, err := netConn.Read(buf[used:])
		if err != nil {
			// 处理读取错误
			if err == io.EOF {
				// 连接正常关闭（对方主动断开）
				log.Debugf("[BASE_LISTENER] 连接正常关闭 (fd=%d)", fd)
			} else {
				// 其他异常错误（如网络中断）
				log.Errorf("[BASE_LISTENER] 读取数据错误 (fd=%d): %v", fd, err)
			}
			// 无论何种错误，均触发断开事件并退出
			s.handler.onDisconnectEvent(fd)
			return
		}

		if n == 0 { // 读取到0字节表示连接关闭
			s.handler.onDisconnectEvent(fd)
			return
		}

		used += n // 更新已使用字节数

		// 处理缓冲区中的数据包
		processed := s.handler.processPackets(fd, &netConn, buf, used)
		if processed > 0 {
			// 将未处理的数据移到缓冲区头部
			used -= processed
			if used > 0 {
				copy(buf, buf[processed:processed+used])
			}
		} else if processed < 0 {
			// 处理失败，关闭连接
			s.handler.onDisconnectEvent(fd)
			log.Errorf("[BASE_LISTENER] 数据包处理失败，关闭连接 (fd=%d)", fd)
			return
		}

		// 检查缓冲区是否溢出
		if used >= len(buf) {
			s.handler.onDisconnectEvent(fd)
			log.Errorf("[BASE_LISTENER] 缓冲区溢出，关闭连接 (fd=%d)", fd)
			return
		}
	}
}

// GetPort 返回服务器监听的端口
// 返回：
//   - 端口号（未启动时返回-1）
func (s *BaseServer) GetPort() int {
	if s.listener == nil {
		return -1
	}
	addr := s.listener.Addr().(*net.TCPAddr)
	return addr.Port
}

func (s *BaseServer) SendBytes(fd int, data []byte) error {
	if conn, ok := s.getConn(fd).(*net.TCPConn); ok {
		_, err := conn.Write(data)
		return err
	}
	return errors.New("无效的连接")
}
