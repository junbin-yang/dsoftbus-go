package authentication

import (
	"fmt"
	"sync"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// ============================================================================
// Auth Channel机制
//
// 这是一个发布-订阅模式的消息路由层，位于auth_tcp_connection之上。
// 主要作用：
// 1. 解耦认证层（authentication）和传输层（transmission）
// 2. 支持多个模块在同一TCP连接上复用
// 3. 根据module字段将消息路由到对应的监听器
//
// 架构：
//   transmission层（trans_auth_manager）
//         ↓ RegAuthChannelListener
//   auth_channel（消息路由）
//         ↓ NotifyChannelDataReceived
//   auth_tcp_connection（TCP协议）
//         ↓ onTcpDataReceived
//   tcp_connection（底层网络）
// ============================================================================

// InnerChannelListener 内部监听器存储结构
type InnerChannelListener struct {
	Module   int32                 // 模块ID
	Listener *AuthChannelListener  // 监听器
}

var (
	g_channelListeners   []InnerChannelListener // 监听器列表
	g_channelListenersMu sync.RWMutex           // 监听器列表锁
)

func init() {
	// 初始化监听器列表，预分配常用的两个模块位置
	// MODULE_AUTH_CHANNEL(8) - 通道控制消息
	// MODULE_AUTH_MSG(9) - 传输数据消息
	g_channelListeners = []InnerChannelListener{
		{Module: ModuleAuthChannel, Listener: nil},
		{Module: ModuleAuthMsg, Listener: nil},
	}
}

// ============================================================================
// 监听器注册管理
// ============================================================================

// RegAuthChannelListener 注册Auth通道监听器
// 对应C函数: int32_t RegAuthChannelListener(int32_t module, const AuthChannelListener *listener)
// 参数:
//   - module: 模块ID（ModuleAuthChannel或ModuleAuthMsg）
//   - listener: 监听器
// 返回:
//   - 错误信息
func RegAuthChannelListener(module int32, listener *AuthChannelListener) error {
	if listener == nil {
		return fmt.Errorf("listener is nil")
	}
	if listener.OnDataReceived == nil {
		return fmt.Errorf("onDataReceived callback is nil")
	}

	g_channelListenersMu.Lock()
	defer g_channelListenersMu.Unlock()

	// 查找是否已存在该模块的监听器
	for i := range g_channelListeners {
		if g_channelListeners[i].Module == module {
			g_channelListeners[i].Listener = listener
			log.Infof("[AUTH_CHANNEL] Registered listener for module=%d", module)
			return nil
		}
	}

	// 不存在则添加新的监听器
	g_channelListeners = append(g_channelListeners, InnerChannelListener{
		Module:   module,
		Listener: listener,
	})
	log.Infof("[AUTH_CHANNEL] Registered new listener for module=%d", module)
	return nil
}

// UnregAuthChannelListener 注销Auth通道监听器
// 对应C函数: void UnregAuthChannelListener(int32_t module)
// 参数:
//   - module: 模块ID
func UnregAuthChannelListener(module int32) {
	g_channelListenersMu.Lock()
	defer g_channelListenersMu.Unlock()

	for i := range g_channelListeners {
		if g_channelListeners[i].Module == module {
			g_channelListeners[i].Listener = nil
			log.Infof("[AUTH_CHANNEL] Unregistered listener for module=%d", module)
			return
		}
	}
}

// ============================================================================
// 通道操作（对SocketConnectDevice/SocketDisconnectDevice的封装）
// ============================================================================

// AuthOpenChannel 打开认证通道（阻塞模式）
// 对应C函数: int32_t AuthOpenChannel(const char *ip, int32_t port)
// 参数:
//   - ip: 远程IP地址
//   - port: 远程端口
// 返回:
//   - 通道ID（成功时返回fd，失败返回InvalidChannelId）
func AuthOpenChannel(ip string, port int) int {
	if ip == "" || port <= 0 {
		log.Errorf("[AUTH_CHANNEL] Invalid parameters: ip=%s, port=%d", ip, port)
		return InvalidChannelId
	}

	fd, err := SocketConnectDevice(ip, port)
	if err != nil {
		log.Errorf("[AUTH_CHANNEL] Failed to connect: %v", err)
		return InvalidChannelId
	}

	log.Infof("[AUTH_CHANNEL] Channel opened: channelId=%d, ip=%s, port=%d", fd, ip, port)
	return fd
}

// AuthCloseChannel 关闭认证通道
// 对应C函数: void AuthCloseChannel(int32_t channelId)
// 参数:
//   - channelId: 通道ID（实际是fd）
func AuthCloseChannel(channelId int) {
	if channelId < 0 {
		log.Debugf("[AUTH_CHANNEL] Invalid channelId: %d", channelId)
		return
	}

	log.Infof("[AUTH_CHANNEL] Closing channel: channelId=%d", channelId)
	SocketDisconnectDevice(Auth, channelId)
}

// ============================================================================
// 数据发送
// ============================================================================

// AuthPostChannelData 发送通道数据
// 对应C函数: int32_t AuthPostChannelData(int32_t channelId, const AuthChannelData *data)
// 参数:
//   - channelId: 通道ID
//   - data: 通道数据
// 返回:
//   - 错误信息
func AuthPostChannelData(channelId int, data *AuthChannelData) error {
	if channelId < 0 || data == nil || data.Data == nil || data.Len == 0 {
		return fmt.Errorf("invalid parameters: channelId=%d", channelId)
	}

	// 构造AuthDataHead
	head := &AuthDataHead{
		DataType: DataTypeConnection, // Auth Channel固定使用CONNECTION类型
		Module:   data.Module,
		Seq:      data.Seq,
		Flag:     data.Flag,
		Len:      data.Len,
	}

	log.Infof("[AUTH_CHANNEL] Posting channel data: channelId=%d, module=%d, seq=%d, len=%d",
		channelId, data.Module, data.Seq, data.Len)

	return SocketPostBytes(channelId, head, data.Data)
}

// ============================================================================
// 内部回调函数（由auth_tcp_connection调用）
// ============================================================================

// NotifyChannelDataReceived 通知通道数据接收
// 由auth_tcp_connection.processSocketData调用
// 根据module字段路由消息到对应的监听器
// 参数:
//   - channelId: 通道ID（fd）
//   - pktHead: 数据包头部
//   - data: 数据内容
func NotifyChannelDataReceived(channelId int, pktHead *SocketPktHead, data []byte) {
	g_channelListenersMu.RLock()
	defer g_channelListenersMu.RUnlock()

	// 查找对应模块的监听器
	var listener *AuthChannelListener
	for i := range g_channelListeners {
		if g_channelListeners[i].Module == pktHead.Module {
			listener = g_channelListeners[i].Listener
			break
		}
	}

	if listener == nil || listener.OnDataReceived == nil {
		log.Warnf("[AUTH_CHANNEL] No listener for module=%d, channelId=%d", pktHead.Module, channelId)
		return
	}

	// 转换为AuthChannelData
	channelData := &AuthChannelData{
		Module: pktHead.Module,
		Seq:    pktHead.Seq,
		Flag:   pktHead.Flag,
		Len:    pktHead.Len,
		Data:   data,
	}

	log.Debugf("[AUTH_CHANNEL] Dispatching data to module=%d listener: channelId=%d, seq=%d, len=%d",
		pktHead.Module, channelId, pktHead.Seq, pktHead.Len)

	// 调用监听器回调
	listener.OnDataReceived(channelId, channelData)
}

// NotifyChannelDisconnected 通知通道断开
// 由auth_tcp_connection.onTcpDisconnected调用
// 通知所有注册的监听器连接已断开
// 参数:
//   - channelId: 通道ID（fd）
func NotifyChannelDisconnected(channelId int) {
	g_channelListenersMu.RLock()
	defer g_channelListenersMu.RUnlock()

	log.Infof("[AUTH_CHANNEL] Notifying disconnect: channelId=%d", channelId)

	// 通知所有注册的监听器
	for i := range g_channelListeners {
		if g_channelListeners[i].Listener != nil && g_channelListeners[i].Listener.OnDisconnected != nil {
			log.Debugf("[AUTH_CHANNEL] Notifying module=%d about disconnect: channelId=%d",
				g_channelListeners[i].Module, channelId)
			g_channelListeners[i].Listener.OnDisconnected(channelId)
		}
	}
}
