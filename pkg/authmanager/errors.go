package authmanager

import "errors"

var (
	ErrInvalidKeyLength      = errors.New("密钥长度无效")
	ErrConnectionClosed      = errors.New("连接已关闭")
	ErrInvalidPacket         = errors.New("无效的数据包")
	ErrBufferTooSmall        = errors.New("缓冲区过小")
	ErrSessionNotFound       = errors.New("会话不存在")
	ErrSessionKeyNotFound    = errors.New("会话密钥不存在")
	ErrMaxConnectionsReached = errors.New("已达最大连接数")
	ErrDecryptionFailed      = errors.New("解密失败")
	ErrInvalidMessage        = errors.New("无效的消息")
)
