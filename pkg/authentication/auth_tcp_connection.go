package authentication

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/tcp_connection"
)

// ============================================================================
// 全局变量
// ============================================================================

var (
	g_connectionManager *tcp_connection.ConnectionManager // 底层连接管理器
	g_socketServer      *tcp_connection.BaseServer        // Socket服务端
	g_socketCallback    *SocketCallback                   // Socket回调
	g_callbackMutex     sync.RWMutex                      // 回调锁
	g_serverMutex       sync.Mutex                        // 服务器锁
)

// ============================================================================
// 数据包编解码
// ============================================================================

// PackSocketPkt 打包Socket数据包
// 对应C函数: PackSocketPkt(const SocketPktHead *pktHead, const uint8_t *data, uint8_t *buf, uint32_t size)
// 参数:
//   - pktHead: 数据包头部
//   - data: 数据内容
// 返回:
//   - 打包后的完整数据包（头部+数据）
//   - 错误信息
func PackSocketPkt(pktHead *SocketPktHead, data []byte) ([]byte, error) {
	if pktHead == nil {
		return nil, fmt.Errorf("pktHead is nil")
	}
	if data == nil {
		return nil, fmt.Errorf("data is nil")
	}
	if pktHead.Len != uint32(len(data)) {
		return nil, fmt.Errorf("data length mismatch: head.Len=%d, actual=%d", pktHead.Len, len(data))
	}

	totalSize := AuthPktHeadLen + len(data)
	buf := make([]byte, totalSize)
	offset := 0

	// 打包头部（小端序，对应C的SoftBusHtoLl）
	binary.LittleEndian.PutUint32(buf[offset:], pktHead.Magic)
	offset += 4
	binary.LittleEndian.PutUint32(buf[offset:], uint32(pktHead.Module))
	offset += 4
	binary.LittleEndian.PutUint64(buf[offset:], uint64(pktHead.Seq))
	offset += 8
	binary.LittleEndian.PutUint32(buf[offset:], uint32(pktHead.Flag))
	offset += 4
	binary.LittleEndian.PutUint32(buf[offset:], pktHead.Len)
	offset += 4

	// 复制数据
	copy(buf[offset:], data)

	return buf, nil
}

// UnpackSocketPkt 解包Socket数据包头部
// 对应C函数: UnpackSocketPkt(const uint8_t *data, uint32_t len, SocketPktHead *head)
// 参数:
//   - data: 接收到的数据缓冲区
// 返回:
//   - 解析出的数据包头部
//   - 错误信息
func UnpackSocketPkt(data []byte) (*SocketPktHead, error) {
	if len(data) < AuthPktHeadLen {
		return nil, fmt.Errorf("data too short: need %d bytes, got %d", AuthPktHeadLen, len(data))
	}

	head := &SocketPktHead{}
	offset := 0

	// 解包头部（小端序，对应C的SoftBusLtoHl）
	head.Magic = binary.LittleEndian.Uint32(data[offset:])
	offset += 4
	head.Module = int32(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	head.Seq = int64(binary.LittleEndian.Uint64(data[offset:]))
	offset += 8
	head.Flag = int32(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	head.Len = binary.LittleEndian.Uint32(data[offset:])

	return head, nil
}

// ============================================================================
// 回调管理
// ============================================================================

// SetSocketCallback 设置Socket回调
// 对应C函数: int32_t SetSocketCallback(const SocketCallback *cb)
func SetSocketCallback(cb *SocketCallback) error {
	if cb == nil {
		return fmt.Errorf("callback is nil")
	}

	g_callbackMutex.Lock()
	defer g_callbackMutex.Unlock()
	g_socketCallback = cb

	log.Info("[AUTH_TCP] Socket callback set successfully")
	return nil
}

// UnsetSocketCallback 取消Socket回调
// 对应C函数: void UnsetSocketCallback(void)
func UnsetSocketCallback() {
	g_callbackMutex.Lock()
	defer g_callbackMutex.Unlock()
	g_socketCallback = nil

	log.Info("[AUTH_TCP] Socket callback unset")
}

// ============================================================================
// 内部回调函数（适配tcp_connection包）
// ============================================================================

// onTcpConnected tcp_connection的OnConnected回调适配
func onTcpConnected(fd int, connType tcp_connection.ConnectionType, connOpt *tcp_connection.ConnectOption) {
	isClient := (connType == tcp_connection.ConnectionTypeClient)

	log.Infof("[AUTH_TCP] OnConnected: fd=%d, isClient=%v", fd, isClient)

	g_callbackMutex.RLock()
	cb := g_socketCallback
	g_callbackMutex.RUnlock()

	if cb != nil && cb.OnConnected != nil {
		// 默认模块为Auth
		cb.OnConnected(Auth, fd, isClient)
	}
}

// onTcpDisconnected tcp_connection的OnDisconnected回调适配
func onTcpDisconnected(fd int, connType tcp_connection.ConnectionType) {
	log.Infof("[AUTH_TCP] OnDisconnected: fd=%d", fd)

	// 先调用SocketCallback
	g_callbackMutex.RLock()
	cb := g_socketCallback
	g_callbackMutex.RUnlock()

	if cb != nil && cb.OnDisconnected != nil {
		cb.OnDisconnected(fd)
	}

	// 再通知Auth Channel层（所有注册的监听器）
	NotifyChannelDisconnected(fd)
}

// onTcpDataReceived tcp_connection的OnDataReceived回调适配
func onTcpDataReceived(fd int, connType tcp_connection.ConnectionType, buf []byte, used int) int {
	processed := 0

	// 循环处理缓冲区中的所有完整数据包
	for processed+AuthPktHeadLen <= used {
		// 解析数据包头部
		head, err := UnpackSocketPkt(buf[processed:])
		if err != nil {
			log.Errorf("[AUTH_TCP] Failed to unpack packet head: %v", err)
			return -1 // 解析失败，关闭连接
		}

		// 验证魔数
		if head.Magic != MagicNumber {
			log.Errorf("[AUTH_TCP] Invalid magic number: 0x%X, expected 0x%X", head.Magic, MagicNumber)
			return -1 // 魔数错误，关闭连接
		}

		// 检查数据长度
		if head.Len == 0 || head.Len > uint32(AuthSocketMaxDataLen) {
			log.Warnf("[AUTH_TCP] Data length out of range: %d", head.Len)
			return -1
		}

		totalPktSize := AuthPktHeadLen + int(head.Len)

		// 检查数据包是否完整
		if processed+totalPktSize > used {
			// 数据包不完整，等待更多数据
			break
		}

		// 提取数据部分
		dataStart := processed + AuthPktHeadLen
		dataEnd := dataStart + int(head.Len)
		data := buf[dataStart:dataEnd]

		log.Infof("[AUTH_TCP] Received packet: fd=%d, module=%d, seq=%d, flag=%d, len=%d",
			fd, head.Module, head.Seq, head.Flag, head.Len)

		// 调用上层回调处理数据
		processSocketData(fd, head, data)

		// 更新已处理字节数
		processed += totalPktSize
	}

	return processed
}

// processSocketData 处理接收到的Socket数据
// 根据module字段路由消息到不同的处理器：
// - MODULE_AUTH_CHANNEL(8)、MODULE_AUTH_MSG(9) -> Auth Channel层
// - MODULE_META_AUTH(21) -> Meta Auth层（暂未实现）
// - 其他模块 -> SocketCallback回调
func processSocketData(fd int, pktHead *SocketPktHead, data []byte) {
	// 路由1: Auth Channel消息（MODULE_AUTH_CHANNEL或MODULE_AUTH_MSG）
	if pktHead.Module == ModuleAuthChannel || pktHead.Module == ModuleAuthMsg {
		log.Debugf("[AUTH_TCP] Routing to Auth Channel: fd=%d, module=%d", fd, pktHead.Module)
		NotifyChannelDataReceived(fd, pktHead, data)
		return
	}

	// 路由2: Meta Auth消息（MODULE_META_AUTH）
	if pktHead.Module == ModuleMetaAuth {
		log.Warnf("[AUTH_TCP] Meta Auth not implemented yet: fd=%d, module=%d", fd, pktHead.Module)
		// TODO: 实现Meta Auth支持
		// AuthMetaNotifyDataReceived(fd, pktHead, data)
		return
	}

	// 路由3: 其他模块走通用SocketCallback路径
	authHead := &AuthDataHead{
		DataType: ModuleToDataType(pktHead.Module),
		Module:   pktHead.Module,
		Seq:      pktHead.Seq,
		Flag:     pktHead.Flag,
		Len:      pktHead.Len,
	}

	g_callbackMutex.RLock()
	cb := g_socketCallback
	g_callbackMutex.RUnlock()

	if cb != nil && cb.OnDataReceived != nil {
		log.Debugf("[AUTH_TCP] Routing to SocketCallback: fd=%d, module=%d", fd, pktHead.Module)
		cb.OnDataReceived(Auth, fd, authHead, data)
	}
}

// ============================================================================
// 服务端监听
// ============================================================================

// StartSocketListening 启动Socket监听
// 对应C函数: int32_t StartSocketListening(ListenerModule module, const LocalListenerInfo *info)
// 参数:
//   - module: 监听器模块类型
//   - ip: 监听IP地址
//   - port: 监听端口（0表示自动分配）
// 返回:
//   - 实际监听端口
//   - 错误信息
func StartSocketListening(module ListenerModule, ip string, port int) (int, error) {
	g_serverMutex.Lock()
	defer g_serverMutex.Unlock()

	if g_socketServer != nil {
		return -1, fmt.Errorf("socket server already started")
	}

	// 创建连接管理器
	if g_connectionManager == nil {
		g_connectionManager = tcp_connection.NewConnectionManager()
	}

	// 创建回调适配器
	tcpCallback := &tcp_connection.BaseListenerCallback{
		OnConnected:    onTcpConnected,
		OnDisconnected: onTcpDisconnected,
		OnDataReceived: onTcpDataReceived,
	}

	// 创建服务端
	g_socketServer = tcp_connection.NewBaseServer(g_connectionManager)

	opt := &tcp_connection.SocketOption{
		Addr: ip,
		Port: port,
	}

	err := g_socketServer.StartBaseListener(opt, tcpCallback)
	if err != nil {
		g_socketServer = nil
		return -1, fmt.Errorf("failed to start listener: %w", err)
	}

	actualPort := g_socketServer.GetPort()
	log.Infof("[AUTH_TCP] Socket listening started: module=%d, addr=%s, port=%d", module, ip, actualPort)

	return actualPort, nil
}

// StopSocketListening 停止Socket监听
// 对应C函数: void StopSocketListening(void)
func StopSocketListening() {
	g_serverMutex.Lock()
	defer g_serverMutex.Unlock()

	if g_socketServer != nil {
		log.Info("[AUTH_TCP] Stopping socket listening...")
		g_socketServer.StopBaseListener()
		g_socketServer = nil
		log.Info("[AUTH_TCP] Socket listening stopped")
	}
}

// ============================================================================
// 客户端连接
// ============================================================================

// SocketConnectDevice 连接设备（阻塞模式）
// 对应C函数: int32_t SocketConnectDevice(const char *ip, int32_t port, bool isBlockMode)
// 参数:
//   - ip: 远程IP地址
//   - port: 远程端口
// 返回:
//   - 文件描述符（虚拟fd）
//   - 错误信息
func SocketConnectDevice(ip string, port int) (int, error) {
	if ip == "" || port <= 0 {
		return AuthInvalidFd, fmt.Errorf("invalid parameters: ip=%s, port=%d", ip, port)
	}

	// 创建连接管理器
	if g_connectionManager == nil {
		g_connectionManager = tcp_connection.NewConnectionManager()
	}

	// 创建回调适配器
	tcpCallback := &tcp_connection.BaseListenerCallback{
		OnConnected:    onTcpConnected,
		OnDisconnected: onTcpDisconnected,
		OnDataReceived: onTcpDataReceived,
	}

	// 创建客户端
	client := tcp_connection.NewBaseClient(g_connectionManager, tcpCallback)

	// 配置连接选项（启用KeepAlive）
	opt := &tcp_connection.ClientOption{
		RemoteIP:        ip,
		RemotePort:      port,
		Timeout:         5 * time.Second,
		KeepAlive:       true,
		KeepAlivePeriod: time.Duration(AuthKeepAliveInterval) * time.Second,
	}

	fd, err := client.Connect(opt)
	if err != nil {
		return AuthInvalidFd, fmt.Errorf("failed to connect to %s:%d: %w", ip, port, err)
	}

	log.Infof("[AUTH_TCP] Connected to device: ip=%s, port=%d, fd=%d", ip, port, fd)
	return fd, nil
}

// SocketDisconnectDevice 断开设备连接
// 对应C函数: void SocketDisconnectDevice(ListenerModule module, int32_t fd)
// 参数:
//   - module: 监听器模块类型
//   - fd: 文件描述符
func SocketDisconnectDevice(module ListenerModule, fd int) {
	if fd < 0 {
		log.Debugf("[AUTH_TCP] Invalid fd, maybe already closed: fd=%d", fd)
		return
	}

	if g_connectionManager != nil {
		g_connectionManager.UnregisterConn(fd)
		log.Infof("[AUTH_TCP] Disconnected device: module=%d, fd=%d", module, fd)
	}
}

// ============================================================================
// 数据发送
// ============================================================================

// SocketPostBytes 发送数据
// 对应C函数: int32_t SocketPostBytes(int32_t fd, const AuthDataHead *head, const uint8_t *data)
// 参数:
//   - fd: 文件描述符
//   - head: 认证数据头部
//   - data: 数据内容
// 返回:
//   - 错误信息
func SocketPostBytes(fd int, head *AuthDataHead, data []byte) error {
	if head == nil || data == nil {
		return fmt.Errorf("invalid parameters: head or data is nil")
	}

	// 转换为SocketPktHead
	pktHead := &SocketPktHead{
		Magic:  MagicNumber,
		Module: head.Module,
		Seq:    head.Seq,
		Flag:   head.Flag,
		Len:    head.Len,
	}

	// 打包数据
	buf, err := PackSocketPkt(pktHead, data)
	if err != nil {
		return fmt.Errorf("failed to pack socket packet: %w", err)
	}

	// 发送数据
	if g_connectionManager == nil {
		return fmt.Errorf("connection manager not initialized")
	}

	log.Infof("[AUTH_TCP] Sending data: fd=%d, module=%d, seq=%d, flag=%d, len=%d",
		fd, pktHead.Module, pktHead.Seq, pktHead.Flag, pktHead.Len)

	err = g_connectionManager.SendBytes(fd, buf)
	if err != nil {
		return fmt.Errorf("failed to send data: %w", err)
	}

	return nil
}

// ============================================================================
// 连接信息查询
// ============================================================================

// SocketGetConnInfo 获取连接信息
// 对应C函数: int32_t SocketGetConnInfo(int32_t fd, AuthConnInfo *connInfo, bool *isServer)
// 参数:
//   - fd: 文件描述符
// 返回:
//   - 连接信息
//   - 是否为服务端
//   - 错误信息
func SocketGetConnInfo(fd int) (*AuthConnInfo, bool, error) {
	if g_connectionManager == nil {
		return nil, false, fmt.Errorf("connection manager not initialized")
	}

	connOpt := g_connectionManager.GetConnInfo(fd)
	if connOpt == nil {
		return nil, false, fmt.Errorf("connection not found: fd=%d", fd)
	}

	// 获取连接类型（判断是否为服务端）
	connType, ok := g_connectionManager.GetConnType(fd)
	if !ok {
		return nil, false, fmt.Errorf("failed to get connection type: fd=%d", fd)
	}
	isServer := (connType == tcp_connection.ConnectionTypeServer)

	// 构造AuthConnInfo
	connInfo := &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   connOpt.RemoteSocket.Addr,
		Port: connOpt.RemoteSocket.Port,
	}

	return connInfo, isServer, nil
}
