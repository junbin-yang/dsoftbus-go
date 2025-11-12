package coap

import (
	"encoding/binary"
	"errors"
	"sync"
)

// COAP_VERSION 定义CoAP版本
const COAP_VERSION = 1

// 消息参数结构体
type COAP_PacketParam struct {
	Protocol   COAP_ProtocolTypeEnum // 协议类型（COAP_UDP/COAP_TCP）
	Type       COAP_TypeEnum         // 消息类型（CON/NONCON/ACK/RESET）
	Code       COAP_MethodTypeEnum   // 方法码（GET/POST等）
	MsgId      uint16                // 消息ID
	Options    []COAP_Option         // 选项列表
	OptionsNum uint8                 // 选项数量
}

// 读写缓冲区
type COAP_ReadWriteBuffer struct {
	Buffer []byte // 实际缓冲区
	Len    int    // 当前已使用长度
	Cap    int    // 缓冲区总容量
}

// 初始化读写缓冲区
func NewCOAPReadWriteBuffer(cap int) *COAP_ReadWriteBuffer {
	return &COAP_ReadWriteBuffer{
		Buffer: make([]byte, cap),
		Len:    0,
		Cap:    cap,
	}
}

// 向缓冲区写入字节（内部辅助函数）
func (buf *COAP_ReadWriteBuffer) writeByte(b byte) error {
	if buf.Len >= buf.Cap {
		return errors.New("buff invalid small")
	}
	buf.Buffer[buf.Len] = b
	buf.Len++
	return nil
}

// 向缓冲区写入字节切片（内部辅助函数）
func (buf *COAP_ReadWriteBuffer) writeBytes(data []byte) error {
	if buf.Len+len(data) > buf.Cap {
		return errors.New("buff invalid small")
	}
	copy(buf.Buffer[buf.Len:], data)
	buf.Len += len(data)
	return nil
}

var (
	g_msgId uint16     = 0
	mu      sync.Mutex // 保护消息ID的互斥锁
)

// 初始化消息ID生成器（简单实现）
func COAP_SoftBusInitMsgId() {
	// 实际项目中需实现消息ID的自增/随机生成逻辑（确保唯一性）
	mu.Lock()
	defer mu.Unlock()
	g_msgId = 0
}

func COAP_SoftBusMsgId() uint16 {
	mu.Lock()
	defer mu.Unlock()

	g_msgId++
	if g_msgId == 0 {
		g_msgId++
	}
	return g_msgId
}

// 解码CoAP数据包（从字节流解析为COAP_Packet）
func COAP_SoftBusDecode(pkt *COAP_Packet, buf []byte, bufLen int) int {
	if pkt == nil || buf == nil || bufLen < 4 { // 头部至少4字节
		return DISCOVERY_ERR_HEADER_INVALID_SHORT
	}

	// 解析头部第一字节（版本+类型+Token长度）
	firstByte := buf[0]
	pkt.Header.Ver = (firstByte >> 6) & 0x03 // 高2位：版本
	if pkt.Header.Ver != COAP_VERSION {
		return DISCOVERY_ERR_VER_INVALID
	}

	pkt.Header.Type = (firstByte >> 4) & 0x03 // 中2位：消息类型
	pkt.Header.TokenLen = firstByte & 0x0F    // 低4位：Token长度
	if pkt.Header.TokenLen > 8 {              // Token最大8字节（RFC规定）
		return DISCOVERY_ERR_TOKEN_INVALID_SHORT
	}

	// 解析代码和消息ID
	pkt.Header.Code = COAP_MethodTypeEnum(buf[1])
	pkt.Header.MsgId = binary.BigEndian.Uint16(buf[2:4])

	// 解析Token（从第4字节开始）
	tokenEnd := 4 + int(pkt.Header.TokenLen)
	if tokenEnd > bufLen {
		return DISCOVERY_ERR_TOKEN_INVALID_SHORT
	}
	pkt.Token.Buffer = buf[4:tokenEnd]
	pkt.Token.Len = uint32(pkt.Header.TokenLen)

	// 解析选项和负载（从Token结束位置开始）
	return parseOptionsAndPayload(pkt, buf, bufLen, tokenEnd)
}

// 解析选项和负载（支持扩展delta/len：13、14）
func parseOptionsAndPayload(pkt *COAP_Packet, buf []byte, bufLen, offset int) int {
	if offset >= bufLen {
		return DISCOVERY_ERR_SUCCESS
	}

	prevOptionNum := uint16(0)
	pkt.OptionsNum = 0

	for offset < bufLen {
		// payload marker
		if buf[offset] == 0xFF {
			offset++
			break
		}
		if pkt.OptionsNum >= COAP_MAX_OPTION {
			return DISCOVERY_ERR_OPT_INVALID_BIG
		}
		if offset >= bufLen {
			return DISCOVERY_ERR_INVALID_PKT
		}
		h := buf[offset]
		offset++
		deltaNib := (h >> 4) & 0x0F
		lenNib := h & 0x0F

		// decode extended delta
		delta := int(deltaNib)
		switch deltaNib {
		case 13:
			if offset >= bufLen {
				return DISCOVERY_ERR_OPT_INVALID_LEN
			}
			delta = 13 + int(buf[offset])
			offset++
		case 14:
			if offset+1 >= bufLen {
				return DISCOVERY_ERR_OPT_INVALID_LEN
			}
			delta = 269 + int(binary.BigEndian.Uint16(buf[offset:offset+2]))
			offset += 2
		case 15:
			return DISCOVERY_ERR_INVALID_PKT // 保留值
		}

		// decode extended length
		l := int(lenNib)
		switch lenNib {
		case 13:
			if offset >= bufLen {
				return DISCOVERY_ERR_OPT_INVALID_LEN
			}
			l = 13 + int(buf[offset])
			offset++
		case 14:
			if offset+1 >= bufLen {
				return DISCOVERY_ERR_OPT_INVALID_LEN
			}
			l = 269 + int(binary.BigEndian.Uint16(buf[offset:offset+2]))
			offset += 2
		case 15:
			return DISCOVERY_ERR_INVALID_PKT // 保留值
		}

		optionNum := prevOptionNum + uint16(delta)
		prevOptionNum = optionNum

		if offset+l > bufLen {
			return DISCOVERY_ERR_OPT_INVALID_LEN
		}
		val := buf[offset : offset+l]
		offset += l

		pkt.Options[pkt.OptionsNum] = COAP_Option{Num: optionNum, OptionBuf: val, Len: uint32(l)}
		pkt.OptionsNum++
	}

	if offset < bufLen {
		pkt.Payload.Buffer = buf[offset:bufLen]
		pkt.Payload.Len = uint32(bufLen - offset)
	}
	return DISCOVERY_ERR_SUCCESS
}

// 编码CoAP数据包（从COAP_Packet生成字节流）
func COAP_SoftBusEncode(pkt *COAP_Packet, param *COAP_PacketParam, token *COAP_Buffer,
	payload *COAP_Buffer, buf *COAP_ReadWriteBuffer) int {
	if pkt == nil || param == nil || buf == nil {
		return -1
	}

	// 填充头部信息
	pkt.Header.Ver = COAP_VERSION
	pkt.Header.Type = uint8(param.Type)
	pkt.Header.Code = param.Code
	pkt.Header.MsgId = param.MsgId
	pkt.Header.TokenLen = uint8(token.Len)
	if pkt.Header.TokenLen > 8 {
		return DISCOVERY_ERR_TOKEN_INVALID_SHORT
	}

	// 写入头部第一字节（版本+类型+Token长度）
	firstByte := (pkt.Header.Ver << 6) | (pkt.Header.Type << 4) | pkt.Header.TokenLen
	if err := buf.writeByte(firstByte); err != nil {
		return -1
	}

	// 写入代码和消息ID
	if err := buf.writeByte(byte(pkt.Header.Code)); err != nil {
		return -1
	}
	msgIdBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(msgIdBytes, pkt.Header.MsgId)
	if err := buf.writeBytes(msgIdBytes); err != nil {
		return -1
	}

	// 写入Token
	if err := buf.writeBytes(token.Buffer[:token.Len]); err != nil {
		return -1
	}

	// 写入选项（支持扩展delta和length）
	prevOptionNum := uint16(0)
	for i := 0; i < int(param.OptionsNum); i++ {
		opt := param.Options[i]
		delta := opt.Num - prevOptionNum
		length := opt.Len

		// 编码delta（处理扩展）
		var deltaNib uint8
		var deltaExt []byte
		switch {
		case delta < 13:
			deltaNib = uint8(delta)
		case delta < 13+256: // 13 ≤ delta ≤ 268
			deltaNib = 13
			deltaExt = []byte{uint8(delta - 13)}
		default: // delta ≥ 269
			deltaNib = 14
			ext := make([]byte, 2)
			binary.BigEndian.PutUint16(ext, uint16(delta-269))
			deltaExt = ext
		}

		// 编码length（处理扩展）
		var lenNib uint8
		var lenExt []byte
		switch {
		case length < 13:
			lenNib = uint8(length)
		case length < 13+256: // 13 ≤ length ≤ 268
			lenNib = 13
			lenExt = []byte{uint8(length - 13)}
		default: // length ≥ 269
			lenNib = 14
			ext := make([]byte, 2)
			binary.BigEndian.PutUint16(ext, uint16(length-269))
			lenExt = ext
		}

		// 写入选项头部（deltaNib <<4 | lenNib）
		optHeader := (deltaNib << 4) | lenNib
		if err := buf.writeByte(optHeader); err != nil {
			return -1
		}

		// 写入delta扩展字节（如果有）
		if len(deltaExt) > 0 {
			if err := buf.writeBytes(deltaExt); err != nil {
				return -1
			}
		}

		// 写入length扩展字节（如果有）
		if len(lenExt) > 0 {
			if err := buf.writeBytes(lenExt); err != nil {
				return -1
			}
		}

		// 写入选项值
		if err := buf.writeBytes(opt.OptionBuf[:opt.Len]); err != nil {
			return -1
		}

		prevOptionNum = opt.Num
	}

	// 写入负载（若有）
	if payload.Len > 0 {
		if err := buf.writeByte(0xFF); err != nil { // 负载标记
			return -1
		}
		if err := buf.writeBytes(payload.Buffer[:payload.Len]); err != nil {
			return -1
		}
	}

	pkt.Len = uint32(buf.Len)
	return DISCOVERY_ERR_SUCCESS
}

// 发送编码后的CoAP消息
func COAP_SendEncodedPacket(socket *SocketInfo, buf *COAP_ReadWriteBuffer) (int, error) {
	if socket == nil || socket.Conn == nil || buf == nil || buf.Len == 0 {
		return 0, ErrInvalidParam
	}
	return CoapSocketSend(socket, buf.Buffer[:buf.Len])
}

// COAP_ResponseInfo - 响应信息结构体
type COAP_ResponseInfo struct {
	RespPkt    *COAP_Packet      // 响应数据包
	RespPara   *COAP_PacketParam // 响应参数
	Token      *COAP_Buffer      // Token（原C中为NULL，此处保留扩展）
	TokenLen   uint32            // Token长度
	Payload    []byte            // 负载数据
	PayloadLen uint32            // 负载长度
}

// BuildSendPkt - 构建发送数据包（贴近原C函数参数和逻辑）
// 功能：根据输入的请求包、远程IP、负载，构建编码后的CoAP发送缓冲区
// 参数：
//   - pkt：输入请求包（用于生成响应的基础信息，如消息类型关联）
//   - remoteIp：远程主机IP（用于URI_HOST选项）
//   - pktPayload：负载数据（字符串形式）
//   - sndPktBuff：输出参数，存储编码后的发送缓冲区
// 返回值：
//   - 错误码（DISCOVERY_ERR_SUCCESS为成功，其他为错误）
func BuildSendPkt(pkt *COAP_Packet, remoteIp string, pktPayload string, sndPktBuff *COAP_ReadWriteBuffer) int {
	// 1. 参数校验
	if pkt == nil || remoteIp == "" || pktPayload == "" || sndPktBuff == nil || sndPktBuff.Buffer == nil {
		return DISCOVERY_ERR_BAD_REQ
	}

	// 2. 初始化响应包和参数
	var respPkt COAP_Packet
	var respPktPara COAP_PacketParam
	options := [COAP_MAX_OPTION]COAP_Option{} // 栈上分配选项数组

	respPktPara.Protocol = COAP_UDP      // 默认为UDP
	respPktPara.Type = COAP_TYPE_NONCON  // COAP_TYPE_ACK     // 响应类型
	respPktPara.Code = pkt.Header.Code   // 复用请求的Code（或根据需求调整）
	respPktPara.MsgId = pkt.Header.MsgId // 复用请求的消息ID（确保关联）
	respPktPara.Options = options[:]     // 绑定选项数组
	respPktPara.OptionsNum = 0           // 初始选项数量

	// 3. 构建选项（URI_HOST和URI_PATH）
	// 3.1 添加URI_HOST选项（值为remoteIp）
	respPktPara.Options[respPktPara.OptionsNum] = COAP_Option{
		Num:       DISCOVERY_MSG_URI_HOST,
		OptionBuf: []byte(remoteIp),
		Len:       uint32(len(remoteIp)),
	}
	respPktPara.OptionsNum++

	// 3.2 添加URI_PATH选项（固定为"device_discover"）
	uriPath := COAP_DEVICE_DISCOVER_URI
	respPktPara.Options[respPktPara.OptionsNum] = COAP_Option{
		Num:       DISCOVERY_MSG_URI_PATH,
		OptionBuf: []byte(uriPath),
		Len:       uint32(len(uriPath)),
	}
	respPktPara.OptionsNum++

	// 4. 初始化响应信息（负载和Token）
	payloadBytes := []byte(pktPayload)
	// 追加null字符（\0），确保C端能正确识别字符串结束
	payloadBytes = append(payloadBytes, 0)
	respInfo := COAP_ResponseInfo{
		RespPkt:    &respPkt,
		RespPara:   &respPktPara,
		Token:      &pkt.Token, // 复用请求的Token（CoAP规范要求）
		TokenLen:   pkt.Token.Len,
		Payload:    payloadBytes,
		PayloadLen: uint32(len(payloadBytes)),
	}

	// 5. 清空发送缓冲区
	sndPktBuff.Len = 0
	if len(sndPktBuff.Buffer) > 0 {
		// 仅清空已分配的缓冲区（避免越界）
		copy(sndPktBuff.Buffer, make([]byte, min(len(sndPktBuff.Buffer), COAP_MAX_PDU_SIZE)))
	}

	// 6. 编码消息
	// 计算缓冲区可用长度（不超过COAP_MAX_PDU_SIZE）
	bufLen := uint32(min(len(sndPktBuff.Buffer), COAP_MAX_PDU_SIZE))
	ret := COAP_SoftBusBuildMessage(pkt, &respInfo, sndPktBuff.Buffer, &bufLen)
	if ret != DISCOVERY_ERR_SUCCESS {
		return DISCOVERY_ERR_BAD_REQ
	}

	// 7. 检查编码后长度是否超出缓冲区容量
	if bufLen >= uint32(sndPktBuff.Cap) {
		return DISCOVERY_ERR_BAD_REQ
	}
	sndPktBuff.Len = int(bufLen)

	return DISCOVERY_ERR_SUCCESS
}

// COAP_SoftBusBuildMessage - 消息构建核心函数
// 功能：整合请求包、响应信息，完成最终编码
func COAP_SoftBusBuildMessage(reqPkt *COAP_Packet, respInfo *COAP_ResponseInfo, buf []byte, len *uint32) int {
	if reqPkt == nil || respInfo == nil || buf == nil || len == nil {
		return DISCOVERY_ERR_BAD_REQ
	}

	// 初始化发送缓冲区包装器
	rwBuf := &COAP_ReadWriteBuffer{
		Buffer: buf,
		Len:    0,
		Cap:    int(*len),
	}

	// 调用编码函数（复用之前的COAP_SoftBusEncode逻辑）
	token := respInfo.Token
	payload := &COAP_Buffer{
		Buffer: respInfo.Payload,
		Len:    respInfo.PayloadLen,
	}
	ret := COAP_SoftBusEncode(
		respInfo.RespPkt,
		respInfo.RespPara,
		token,
		payload,
		rwBuf,
	)
	if ret != DISCOVERY_ERR_SUCCESS {
		return ret
	}

	// 更新输出长度
	*len = uint32(rwBuf.Len)
	return DISCOVERY_ERR_SUCCESS
}

// 辅助函数：取较小值（避免缓冲区越界）
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
