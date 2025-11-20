package tcp_connection

import (
	"fmt"
	"io"
	"net"
	"sync"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// BaseServer 负责监听连接、处理数据收发及连接管理
type BaseServer struct {
	listener net.Listener          // 连接监听器
	connMgr  *ConnectionManager    // 连接管理器
	stopChan chan struct{}         // 用于通知停止的通道
	wg       sync.WaitGroup        // 用于等待所有goroutine结束
	callback *BaseListenerCallback // 统一的事件回调
}

// NewBaseServer 创建新的服务端实例
func NewBaseServer(connMgr *ConnectionManager) *BaseServer {
	if connMgr == nil {
		connMgr = NewConnectionManager()
	}
	return &BaseServer{
		connMgr:  connMgr,
		stopChan: make(chan struct{}),
	}
}

// StartBaseListener 启动基础监听器
// 参数：
//   - opt：本地监听器的配置信息（包含地址、端口等）
//   - callback：统一的事件回调处理器
func (s *BaseServer) StartBaseListener(opt *SocketOption, callback *BaseListenerCallback) error {
	if opt == nil || callback == nil {
		return fmt.Errorf("参数错误")
	}

	// 创建监听器
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", opt.Addr, opt.Port))
	if err != nil {
		return fmt.Errorf("创建监听器失败：%v", err)
	}

	s.listener = listener
	s.callback = callback
	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// StopBaseListener 停止基础监听器
func (s *BaseServer) StopBaseListener() error {
	close(s.stopChan)

	if s.listener != nil {
		// 关闭监听器
		s.listener.Close()
	}

	// 关闭所有服务端连接
	fds := s.connMgr.GetAllFds()
	for _, fd := range fds {
		if connType, ok := s.connMgr.GetConnType(fd); ok && connType == ConnectionTypeServer {
			s.connMgr.UnregisterConn(fd)
		}
	}

	s.wg.Wait()
	return nil
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
				log.Errorf("[BASE_SERVER] 接受连接错误: %v", err)
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
	localAddr := netConn.LocalAddr().(*net.TCPAddr)

	// 注册连接并获取虚拟文件描述符
	fd := s.connMgr.RegisterConn(netConn, ConnectionTypeServer)
	defer s.connMgr.UnregisterConn(fd) // 退出时注销连接

	log.Debugf("[BASE_SERVER] 新连接来自 %s:%d (fd=%d)", remoteAddr.IP.String(), remoteAddr.Port, fd)

	// 触发连接事件回调
	if s.callback != nil && s.callback.OnConnected != nil {
		connectInfo := &ConnectOption{
			LocalSocket: &SocketOption{
				Addr: localAddr.IP.String(),
				Port: localAddr.Port,
			},
			RemoteSocket: &SocketOption{
				Addr: remoteAddr.IP.String(),
				Port: remoteAddr.Port,
			},
			NetConn: &netConn,
		}
		s.callback.OnConnected(fd, ConnectionTypeServer, connectInfo)
	}

	// 处理该连接的数据事件
	s.handleDataEvents(fd, netConn)
}

// handleDataEvents 处理连接的数据接收和处理
// 参数：
//   - fd：虚拟文件描述符
//   - netConn：网络连接
func (s *BaseServer) handleDataEvents(fd int, netConn net.Conn) {
	buf := make([]byte, DefaultBufSize)
	used := 0 // 缓冲区中已使用的字节数

	for {
		select {
		case <-s.stopChan: // 收到停止信号
			if s.callback != nil && s.callback.OnDisconnected != nil {
				s.callback.OnDisconnected(fd, ConnectionTypeServer)
			}
			return
		default:
		}

		// 从连接读取数据（阻塞模式，无数据时会等待）
		n, err := netConn.Read(buf[used:])
		if err != nil {
			// 处理读取错误
			if err == io.EOF {
				// 连接正常关闭（对方主动断开）
				log.Debugf("[BASE_SERVER] 连接正常关闭 (fd=%d)", fd)
			} else {
				// 其他异常错误（如网络中断）
				log.Errorf("[BASE_SERVER] 读取数据错误 (fd=%d): %v", fd, err)
			}
			// 无论何种错误，均触发断开事件并退出
			if s.callback != nil && s.callback.OnDisconnected != nil {
				s.callback.OnDisconnected(fd, ConnectionTypeServer)
			}
			return
		}

		if n == 0 { // 读取到0字节表示连接关闭
			if s.callback != nil && s.callback.OnDisconnected != nil {
				s.callback.OnDisconnected(fd, ConnectionTypeServer)
			}
			return
		}

		used += n // 更新已使用字节数

		// 处理缓冲区中的数据包
		if s.callback != nil && s.callback.OnDataReceived != nil {
			processed := s.callback.OnDataReceived(fd, ConnectionTypeServer, buf, used)
			if processed > 0 {
				// 将未处理的数据移到缓冲区头部
				used -= processed
				if used > 0 {
					copy(buf, buf[processed:processed+used])
				}
			} else if processed < 0 {
				// 处理失败，关闭连接
				if s.callback.OnDisconnected != nil {
					s.callback.OnDisconnected(fd, ConnectionTypeServer)
				}
				log.Errorf("[BASE_SERVER] 数据包处理失败，关闭连接 (fd=%d)", fd)
				return
			}
		}

		// 检查缓冲区是否溢出
		if used >= len(buf) {
			if s.callback != nil && s.callback.OnDisconnected != nil {
				s.callback.OnDisconnected(fd, ConnectionTypeServer)
			}
			log.Errorf("[BASE_SERVER] 缓冲区溢出，关闭连接 (fd=%d)", fd)
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

// GetAddr 返回服务器监听的地址
// 返回：
//   - IP地址（未启动时返回空字符串）
func (s *BaseServer) GetAddr() string {
	if s.listener == nil {
		return ""
	}
	addr := s.listener.Addr().(*net.TCPAddr)
	return addr.IP.String()
}

// SendBytes 发送数据到指定连接
// 参数：
//   - fd：虚拟文件描述符
//   - data：要发送的数据
// 返回：
//   - 错误信息（发送失败时）
func (s *BaseServer) SendBytes(fd int, data []byte) error {
	return s.connMgr.SendBytes(fd, data)
}

// GetConnInfo 获取连接的完整信息（包含本地和远程两端）
// 参数：
//   - fd：虚拟文件描述符
// 返回：
//   - 连接完整信息
func (s *BaseServer) GetConnInfo(fd int) *ConnectOption {
	return s.connMgr.GetConnInfo(fd)
}

// GetConnPeerInfo 获取连接的对端信息（已废弃，建议使用 GetConnInfo）
// 参数：
//   - fd：虚拟文件描述符
// 返回：
//   - 对端连接信息
// Deprecated: 使用 GetConnInfo 获取完整的连接信息
func (s *BaseServer) GetConnPeerInfo(fd int) *ConnectOption {
	return s.connMgr.GetConnInfo(fd)
}

// GetConnLocalInfo 获取连接的本地信息（已废弃，建议使用 GetConnInfo）
// 参数：
//   - fd：虚拟文件描述符
// 返回：
//   - 本地连接信息
// Deprecated: 使用 GetConnInfo 获取完整的连接信息
func (s *BaseServer) GetConnLocalInfo(fd int) *ConnectOption {
	return s.connMgr.GetConnInfo(fd)
}
