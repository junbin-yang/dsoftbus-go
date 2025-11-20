package tcp_connection

import (
	"fmt"
	"io"
	"net"
	"sync"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// BaseClient 客户端连接管理器
type BaseClient struct {
	fd       int                    // 虚拟文件描述符
	conn     net.Conn               // 与远程设备的网络连接
	connMgr  *ConnectionManager     // 连接管理器
	callback *BaseListenerCallback  // 统一的事件回调处理器
	stopChan chan struct{}          // 停止信号通道
	wg       sync.WaitGroup         // 等待goroutine退出
	mu       sync.Mutex             // 保护conn和fd的锁
}

// NewBaseClient 创建新的客户端实例
// 参数：
//   - connMgr：连接管理器（如果为nil，则创建新的）
//   - callback：统一的事件回调处理器
func NewBaseClient(connMgr *ConnectionManager, callback *BaseListenerCallback) *BaseClient {
	if connMgr == nil {
		connMgr = NewConnectionManager()
	}
	return &BaseClient{
		connMgr:  connMgr,
		callback: callback,
		stopChan: make(chan struct{}),
		fd:       -1, // 初始化为无效fd
	}
}

// Connect 连接到远程设备（使用ClientOption配置）
// 参数：
//   - opt：客户端配置选项
// 返回：
//   - 虚拟文件描述符（连接成功时）
//   - 错误信息（连接失败时）
func (c *BaseClient) Connect(opt *ClientOption) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return -1, fmt.Errorf("已建立连接")
	}
	if opt == nil {
		return -1, fmt.Errorf("配置选项不能为空")
	}

	// 使用配置的超时时间，如果未设置则使用默认值
	timeout := opt.Timeout
	if timeout == 0 {
		timeout = DefaultConnectTimeout
	}

	addr := fmt.Sprintf("%s:%d", opt.RemoteIP, opt.RemotePort)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return -1, fmt.Errorf("连接到%s失败: %v", addr, err)
	}

	// 配置长连接选项
	if opt.KeepAlive {
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetKeepAlive(true)
			if opt.KeepAlivePeriod > 0 {
				tcpConn.SetKeepAlivePeriod(opt.KeepAlivePeriod)
			}
		}
	}

	// 注册连接并获取虚拟文件描述符
	fd := c.connMgr.RegisterConn(conn, ConnectionTypeClient)
	c.conn = conn
	c.fd = fd

	log.Debugf("[BASE_CLIENT] 连接成功 %s (fd=%d)", addr, fd)

	// 启动数据接收循环
	c.wg.Add(1)
	go c.handleDataEvents()

	// 触发连接成功回调
	if c.callback != nil && c.callback.OnConnected != nil {
		remoteAddr := conn.RemoteAddr().(*net.TCPAddr)
		localAddr := conn.LocalAddr().(*net.TCPAddr)
		connectInfo := &ConnectOption{
			LocalSocket: &SocketOption{
				Addr: localAddr.IP.String(),
				Port: localAddr.Port,
			},
			RemoteSocket: &SocketOption{
				Addr: remoteAddr.IP.String(),
				Port: remoteAddr.Port,
			},
			NetConn: &conn,
		}
		c.callback.OnConnected(fd, ConnectionTypeClient, connectInfo)
	}

	return fd, nil
}

// ConnectSimple 简化的连接方法（兼容旧接口）
// 参数：
//   - remoteIP：远程设备IP地址
//   - remotePort：远程设备端口
// 返回：
//   - 虚拟文件描述符（连接成功时）
//   - 错误信息（连接失败时）
func (c *BaseClient) ConnectSimple(remoteIP string, remotePort int) (int, error) {
	opt := &ClientOption{
		RemoteIP:   remoteIP,
		RemotePort: remotePort,
		Timeout:    DefaultConnectTimeout,
		KeepAlive:  false,
	}
	return c.Connect(opt)
}

// Close 关闭与远程设备的连接
func (c *BaseClient) Close() {
	c.mu.Lock()
	fd := c.fd
	conn := c.conn
	if conn == nil || fd < 0 {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	// 发送停止信号
	select {
	case <-c.stopChan:
		// 已经关闭
		return
	default:
		close(c.stopChan)
	}

	// 关闭网络连接（这会导致 Read 返回错误）
	conn.Close()

	// 等待接收循环退出
	c.wg.Wait()

	// 从管理器注销连接
	c.connMgr.UnregisterConn(fd)

	c.mu.Lock()
	c.conn = nil
	c.fd = -1
	c.mu.Unlock()

	// 触发断开连接回调
	if c.callback != nil && c.callback.OnDisconnected != nil {
		c.callback.OnDisconnected(fd, ConnectionTypeClient)
	}

	log.Debugf("[BASE_CLIENT] 连接已关闭 (fd=%d)", fd)
}

// SendBytes 发送数据
// 参数：
//   - data：要发送的数据
// 返回：
//   - 错误信息（发送失败时）
func (c *BaseClient) SendBytes(data []byte) error {
	c.mu.Lock()
	fd := c.fd
	c.mu.Unlock()

	if fd < 0 {
		return fmt.Errorf("未建立连接")
	}

	return c.connMgr.SendBytes(fd, data)
}

// GetFd 获取虚拟文件描述符
// 返回：
//   - 虚拟文件描述符（未连接时返回-1）
func (c *BaseClient) GetFd() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.fd
}

// IsConnected 检查是否已连接
// 返回：
//   - true：已连接，false：未连接
func (c *BaseClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil && c.fd >= 0
}

// handleDataEvents 循环接收数据并触发回调
func (c *BaseClient) handleDataEvents() {
	defer c.wg.Done()

	buf := make([]byte, DefaultBufSize)
	used := 0 // 缓冲区已使用字节数

	for {
		select {
		case <-c.stopChan: // 收到停止信号，退出循环
			return
		default:
		}

		// 读取数据（阻塞模式，与服务端保持一致）
		c.mu.Lock()
		conn := c.conn
		fd := c.fd
		c.mu.Unlock()

		if conn == nil || fd < 0 {
			return
		}

		n, err := conn.Read(buf[used:])
		if err != nil {
			// 处理读取错误
			if err == io.EOF {
				log.Debugf("[BASE_CLIENT] 连接正常关闭 (fd=%d)", fd)
			} else {
				log.Errorf("[BASE_CLIENT] 读取数据错误 (fd=%d): %v", fd, err)
			}
			// 不在这里调用 Close()，避免死锁，直接返回
			return
		}

		if n == 0 {
			return
		}

		used += n

		// 调用数据处理回调
		if c.callback != nil && c.callback.OnDataReceived != nil {
			processed := c.callback.OnDataReceived(fd, ConnectionTypeClient, buf, used)
			if processed > 0 {
				// 移动未处理数据到缓冲区头部
				used -= processed
				if used > 0 {
					copy(buf, buf[processed:processed+used])
				}
			} else if processed < 0 {
				// 处理失败，关闭连接
				log.Errorf("[BASE_CLIENT] 数据包处理失败，关闭连接 (fd=%d)", fd)
				return
			}
		}

		// 检查缓冲区溢出
		if used >= len(buf) {
			log.Errorf("[BASE_CLIENT] 缓冲区溢出，关闭连接 (fd=%d)", fd)
			return
		}
	}
}

// GetConnInfo 获取连接的完整信息（包含本地和远程两端）
// 返回：
//   - 连接完整信息
func (c *BaseClient) GetConnInfo() *ConnectOption {
	c.mu.Lock()
	fd := c.fd
	c.mu.Unlock()

	if fd < 0 {
		return nil
	}
	return c.connMgr.GetConnInfo(fd)
}

// GetConnPeerInfo 获取连接的对端信息（已废弃，建议使用 GetConnInfo）
// 返回：
//   - 对端连接信息
// Deprecated: 使用 GetConnInfo 获取完整的连接信息
func (c *BaseClient) GetConnPeerInfo() *ConnectOption {
	return c.GetConnInfo()
}

// GetConnLocalInfo 获取连接的本地信息（已废弃，建议使用 GetConnInfo）
// 返回：
//   - 本地连接信息
// Deprecated: 使用 GetConnInfo 获取完整的连接信息
func (c *BaseClient) GetConnLocalInfo() *ConnectOption {
	return c.GetConnInfo()
}
