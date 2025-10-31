package authmanager

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// ModuleUseCipherText 检查指定模块是否需要加密传输数据
// 参数：
//   - module：模块类型（对应ModuleXXX常量）
// 返回：
//   - 是否需要加密（true：需要加密；false：明文传输）
func ModuleUseCipherText(module int) bool {
	// 信任引擎到HiChain同步模块（1-4）不使用加密
	if module >= ModuleTrustEngine && module <= ModuleHiChainSync {
		return false
	}
	// 认证通道到认证消息模块（8-9）不使用加密
	if module >= ModuleAuthChannel && module <= ModuleAuthMsg {
		return false
	}
	// 其他模块需要加密
	return true
}

// AuthConnSend 通过连接发送数据
// 参数：
//   - conn：网络连接
//   - data：待发送的数据
//   - timeout：超时时间（0表示无超时）
// 返回：
//   - 发送的字节数
//   - 错误信息（发送失败时）
func AuthConnSend(conn net.Conn, data []byte, timeout time.Duration) (int, error) {
	if timeout > 0 {
		// 设置写超时
		conn.SetWriteDeadline(time.Now().Add(timeout))
		// 发送后清除超时设置
		defer conn.SetWriteDeadline(time.Time{})
	}
	return conn.Write(data)
}

// AuthConnRecv 从连接接收数据
// 参数：
//   - conn：网络连接
//   - buf：接收缓冲区
//   - timeout：超时时间（0表示无超时）
// 返回：
//   - 接收的字节数
//   - 错误信息（接收失败时）
func AuthConnRecv(conn net.Conn, buf []byte, timeout time.Duration) (int, error) {
	if timeout > 0 {
		// 设置读超时
		conn.SetReadDeadline(time.Now().Add(timeout))
		// 接收后清除超时设置
		defer conn.SetReadDeadline(time.Time{})
	}
	return conn.Read(buf)
}

// AuthConnPackBytes 将数据打包为协议数据包格式
// 功能：根据模块类型决定是否加密，然后添加协议头部
// 参数：
//   - module：模块类型
//   - flags：标志位（如FlagReply）
//   - seqNum：序列号
//   - data：原始数据
//   - skey：会话密钥（加密时使用，非加密模块可传nil）
// 返回：
//   - 打包后的完整数据包
//   - 错误信息（打包失败时）
func AuthConnPackBytes(module int, flags int, seqNum int64, data []byte, skey *SessionKey) ([]byte, error) {
	isCipherText := ModuleUseCipherText(module) // 判断是否需要加密

	var payload []byte
	var err error

	if isCipherText && skey != nil {
		// 需要加密：使用会话密钥加密数据
		payload, err = GetEncryptTransData(seqNum, data, skey)
		if err != nil {
			return nil, err
		}
	} else {
		// 不需要加密：直接使用原始数据作为负载
		payload = data
	}

	dataLen := len(payload)
	totalLen := PacketHeadSize + dataLen // 总长度 = 头部大小 + 负载大小
	buf := make([]byte, totalLen)

	offset := 0
	// 标识符（4字节，固定为PkgHeaderIdentifier）
	binary.LittleEndian.PutUint32(buf[offset:], PkgHeaderIdentifier)
	offset += 4

	// 模块类型（4字节）
	binary.LittleEndian.PutUint32(buf[offset:], uint32(module))
	offset += 4

	// 序列号（8字节）
	binary.LittleEndian.PutUint64(buf[offset:], uint64(seqNum))
	offset += 8

	// 标志位（4字节）
	binary.LittleEndian.PutUint32(buf[offset:], uint32(flags))
	offset += 4

	// 数据长度（4字节，负载部分的长度）
	binary.LittleEndian.PutUint32(buf[offset:], uint32(dataLen))
	offset += 4

	// 负载数据（加密或原始数据）
	copy(buf[offset:], payload)

	return buf, nil
}

// AuthConnPostBytes 通过连接发送原始字节数据（自动打包）
// 功能：先调用AuthConnPackBytes打包，再通过连接发送
// 参数：
//   - conn：网络连接
//   - 其他参数：同AuthConnPackBytes
// 返回：
//   - 错误信息（发送失败时）
func AuthConnPostBytes(conn net.Conn, module int, flags int, seq int64, data []byte, skey *SessionKey) error {
	// 打包数据
	buf, err := AuthConnPackBytes(module, flags, seq, data, skey)
	if err != nil {
		return err
	}

	// 发送数据
	n, err := AuthConnSend(conn, buf, 0)
	if err != nil {
		return err
	}

	// 验证是否发送完整
	if n != len(buf) {
		return fmt.Errorf("发送不完整：已发送%d字节，预期%d字节", n, len(buf))
	}

	return nil
}

// AuthConnPostMessage 通过连接发送JSON消息（自动序列化和打包）
// 功能：先将消息序列化为JSON，再调用AuthConnPostBytes发送
// 参数：
//   - conn：网络连接
//   - msg：要发送的消息（结构体对象）
//   - 其他参数：同AuthConnPackBytes
// 返回：
//   - 错误信息（序列化或发送失败时）
func AuthConnPostMessage(conn net.Conn, module int, flags int, seqNum int64, msg interface{}, skey *SessionKey) error {
	// 将消息序列化为JSON字节流
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// 发送序列化后的数据
	return AuthConnPostBytes(conn, module, flags, seqNum, msgBytes, skey)
}

// ParsePacketHead 从缓冲区解析数据包头部
// 功能：提取并验证头部信息（标识符、模块、序列号等）
// 参数：
//   - buf：数据缓冲区
//   - offset：头部在缓冲区中的起始偏移量
// 返回：
//   - 解析出的Packet结构体（包含头部信息）
//   - 错误信息（解析失败或头部无效时）
func ParsePacketHead(buf []byte, offset int) (*Packet, error) {
	// 验证缓冲区大小是否足够容纳头部
	if len(buf)-offset < PacketHeadSize {
		return nil, fmt.Errorf("缓冲区过小，无法容纳数据包头部")
	}

	pos := offset

	// 验证标识符（必须为PkgHeaderIdentifier）
	identifier := binary.LittleEndian.Uint32(buf[pos:])
	if identifier != PkgHeaderIdentifier {
		return nil, fmt.Errorf("无效的数据包标识符：0x%X", identifier)
	}
	pos += 4

	// 解析模块类型
	module := int(binary.LittleEndian.Uint32(buf[pos:]))
	pos += 4

	// 解析序列号
	seq := int64(binary.LittleEndian.Uint64(buf[pos:]))
	pos += 8

	// 解析标志位
	flags := int(binary.LittleEndian.Uint32(buf[pos:]))
	pos += 4

	// 解析数据长度
	dataLen := int(binary.LittleEndian.Uint32(buf[pos:]))

	// 验证头部参数合法性
	if module < 0 || flags < 0 || dataLen < 0 || dataLen > PacketDataSize {
		return nil, fmt.Errorf("无效的数据包参数")
	}

	return &Packet{
		Module:  module,
		Flags:   flags,
		Seq:     seq,
		DataLen: dataLen,
	}, nil
}
