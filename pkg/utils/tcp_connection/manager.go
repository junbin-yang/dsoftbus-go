package tcp_connection

import (
	"errors"
	"net"
	"sync"
)

// ConnectionManager 统一的连接管理器（管理服务端和客户端的所有连接）
type ConnectionManager struct {
	connMap       map[int]net.Conn      // 连接映射（虚拟fd -> 网络连接）
	connTypeMap   map[int]ConnectionType // 连接类型映射（虚拟fd -> 连接类型）
	mu            sync.RWMutex          // 保护connMap和connTypeMap的读写锁
	serverNextFd  int                   // 服务端下一个可用的虚拟fd
	clientNextFd  int                   // 客户端下一个可用的虚拟fd
	fdMu          sync.Mutex            // 保护fd分配的互斥锁
}

// NewConnectionManager 创建新的连接管理器
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connMap:      make(map[int]net.Conn),
		connTypeMap:  make(map[int]ConnectionType),
		serverNextFd: ServerFdStart,
		clientNextFd: ClientFdStart,
	}
}

// AllocateFd 为连接分配虚拟文件描述符
// 参数：
//   - connType：连接类型（服务端或客户端）
// 返回：
//   - 分配的虚拟文件描述符
func (m *ConnectionManager) AllocateFd(connType ConnectionType) int {
	m.fdMu.Lock()
	defer m.fdMu.Unlock()

	var fd int
	if connType == ConnectionTypeServer {
		fd = m.serverNextFd
		m.serverNextFd++
	} else {
		fd = m.clientNextFd
		m.clientNextFd++
	}
	return fd
}

// RegisterConn 注册连接并分配虚拟文件描述符
// 参数：
//   - conn：网络连接
//   - connType：连接类型（服务端或客户端）
// 返回：
//   - 分配的虚拟文件描述符
func (m *ConnectionManager) RegisterConn(conn net.Conn, connType ConnectionType) int {
	fd := m.AllocateFd(connType)
	m.mu.Lock()
	m.connMap[fd] = conn
	m.connTypeMap[fd] = connType
	m.mu.Unlock()
	return fd
}

// UnregisterConn 注销连接（关闭并从映射中移除）
// 参数：
//   - fd：虚拟文件描述符
func (m *ConnectionManager) UnregisterConn(fd int) {
	m.mu.Lock()
	if conn, ok := m.connMap[fd]; ok {
		conn.Close()
		delete(m.connMap, fd)
		delete(m.connTypeMap, fd)
	}
	m.mu.Unlock()
}

// GetConn 通过虚拟文件描述符获取连接
// 参数：
//   - fd：虚拟文件描述符
// 返回：
//   - 对应的网络连接（net.Conn）
//   - 连接是否存在
func (m *ConnectionManager) GetConn(fd int) (net.Conn, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.connMap[fd]
	return conn, ok
}

// GetConnType 获取连接类型
// 参数：
//   - fd：虚拟文件描述符
// 返回：
//   - 连接类型
//   - 连接是否存在
func (m *ConnectionManager) GetConnType(fd int) (ConnectionType, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	connType, ok := m.connTypeMap[fd]
	return connType, ok
}

// SendBytes 通过指定的虚拟fd发送数据
// 参数：
//   - fd：虚拟文件描述符
//   - data：要发送的数据
// 返回：
//   - 错误信息（发送失败时）
func (m *ConnectionManager) SendBytes(fd int, data []byte) error {
	conn, ok := m.GetConn(fd)
	if !ok {
		return errors.New("无效的连接")
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_, err := tcpConn.Write(data)
		return err
	}
	return errors.New("无效的TCP连接")
}

// GetConnInfo 获取连接的完整信息（包含本地和远程两端）
// 参数：
//   - fd：虚拟文件描述符
// 返回：
//   - 连接完整信息
func (m *ConnectionManager) GetConnInfo(fd int) *ConnectOption {
	conn, ok := m.GetConn(fd)
	if !ok {
		return nil
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		remoteAddr := tcpConn.RemoteAddr().(*net.TCPAddr)
		localAddr := tcpConn.LocalAddr().(*net.TCPAddr)
		return &ConnectOption{
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
	}
	return nil
}

// GetConnPeerInfo 获取连接的对端信息（已废弃，建议使用 GetConnInfo）
// 参数：
//   - fd：虚拟文件描述符
// 返回：
//   - 对端连接信息
// Deprecated: 使用 GetConnInfo 获取完整的连接信息
func (m *ConnectionManager) GetConnPeerInfo(fd int) *ConnectOption {
	return m.GetConnInfo(fd)
}

// GetConnLocalInfo 获取连接的本地信息（已废弃，建议使用 GetConnInfo）
// 参数：
//   - fd：虚拟文件描述符
// 返回：
//   - 本地连接信息
// Deprecated: 使用 GetConnInfo 获取完整的连接信息
func (m *ConnectionManager) GetConnLocalInfo(fd int) *ConnectOption {
	return m.GetConnInfo(fd)
}

// CloseConn 关闭指定的连接（但不从管理器中移除）
// 参数：
//   - fd：虚拟文件描述符
// 返回：
//   - 错误信息（关闭失败时）
func (m *ConnectionManager) CloseConn(fd int) error {
	conn, ok := m.GetConn(fd)
	if !ok {
		return errors.New("连接不存在")
	}
	return conn.Close()
}

// GetAllFds 获取所有活跃的虚拟文件描述符
// 返回：
//   - 所有活跃的虚拟文件描述符列表
func (m *ConnectionManager) GetAllFds() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fds := make([]int, 0, len(m.connMap))
	for fd := range m.connMap {
		fds = append(fds, fd)
	}
	return fds
}

// GetConnCount 获取连接数量
// 返回：
//   - 连接总数
func (m *ConnectionManager) GetConnCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connMap)
}

// CloseAll 关闭所有连接
func (m *ConnectionManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for fd, conn := range m.connMap {
		conn.Close()
		delete(m.connMap, fd)
		delete(m.connTypeMap, fd)
	}
}
