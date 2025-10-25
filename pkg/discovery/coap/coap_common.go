package coap

// 协议类型
type COAP_ProtocolTypeEnum uint8

const (
	COAP_UDP COAP_ProtocolTypeEnum = 0
	COAP_TCP COAP_ProtocolTypeEnum = 1
)

// 方法类型
type COAP_MethodTypeEnum uint8

const (
	COAP_METHOD_GET    COAP_MethodTypeEnum = 1
	COAP_METHOD_POST   COAP_MethodTypeEnum = 2
	COAP_METHOD_PUT    COAP_MethodTypeEnum = 3
	COAP_METHOD_DELETE COAP_MethodTypeEnum = 4
)

// 消息类型
type COAP_TypeEnum uint8

const (
	COAP_TYPE_CON    COAP_TypeEnum = 0
	COAP_TYPE_NONCON COAP_TypeEnum = 1
	COAP_TYPE_ACK    COAP_TypeEnum = 2
	COAP_TYPE_RESET  COAP_TypeEnum = 3
)

// 选项常量
const (
	DISCOVERY_MSG_URI_HOST = 3
	DISCOVERY_MSG_URI_PATH = 11
	COAP_MAX_OPTION        = 16
)

// CoAP头部结构
type COAP_Header struct {
	Ver      uint8               // 2位版本号
	Type     uint8               // 2位消息类型
	TokenLen uint8               // 4位Token长度
	Code     COAP_MethodTypeEnum // 8位代码
	MsgId    uint16              // 16位消息ID
}

// 缓冲区结构
type COAP_Buffer struct {
	Buffer []byte
	Len    uint32
}

// 选项结构
type COAP_Option struct {
	Num       uint16
	OptionBuf []byte
	Len       uint32
}

// 数据包结构
type COAP_Packet struct {
	Protocol   COAP_ProtocolTypeEnum
	Len        uint32
	Header     COAP_Header
	Token      COAP_Buffer
	OptionsNum uint8
	Options    [COAP_MAX_OPTION]COAP_Option
	Payload    COAP_Buffer
}

// 错误类型
const (
	DISCOVERY_ERR_SUCCESS              = 0
	DISCOVERY_ERR_HEADER_INVALID_SHORT = 1
	DISCOVERY_ERR_VER_INVALID          = 2
	DISCOVERY_ERR_TOKEN_INVALID_SHORT  = 3
	//DISCOVERY_ERR_OPT_INVALID_SHORT_FOR_HEADER = 4
	//DISCOVERY_ERR_OPT_INVALID_SHORT            = 5
	//DISCOVERY_ERR_OPT_OVERRUNS_PKT             = 6
	DISCOVERY_ERR_OPT_INVALID_BIG = 7
	DISCOVERY_ERR_OPT_INVALID_LEN = 8
	//DISCOVERY_ERR_BUF_INVALID_SMALL            = 9
	//DISCOVERY_ERR_NOT_SUPPORTED                = 10
	DISCOVERY_ERR_OPT_INVALID_DELTA = 11
	//DISCOVERY_ERR_PKT_EXCEED_MAX_PDU           = 12
	//DISCOVERY_ERR_TCP_TYPE_INVALID             = 13
	//DISCOVERY_ERR_UNKNOWN_MSG_TYPE             = 14
	DISCOVERY_ERR_INVALID_PKT = 15
	//DISCOVERY_ERR_INVALID_TOKEN_LEN            = 16
	//DISCOVERY_ERR_INVALID_ARGUMENT             = 17
	//DISCOVERY_ERR_TRANSPORT_NOT_UDP_OR_TCP     = 18
	//DISCOVERY_ERR_INVALID_EMPTY_MSG            = 19
	//DISCOVERY_ERR_SERVER_ERR                   = 20
	DISCOVERY_ERR_BAD_REQ = 21
	//DISCOVERY_ERR_UNKNOWN_METHOD               = 22
	//DISCOVERY_ERR_BLOCK_NO_PAYLOAD             = 23
)

