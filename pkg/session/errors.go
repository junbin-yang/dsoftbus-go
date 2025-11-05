package session

import "errors"

var (
	// 会话相关错误
	ErrSessionNotFound      = errors.New("session not found")
	ErrSessionExists        = errors.New("session already exists")
	ErrSessionClosed        = errors.New("session is closed")
	ErrInvalidSessionID     = errors.New("invalid session ID")
	ErrInvalidSessionName   = errors.New("invalid session name")

	// 服务器相关错误
	ErrServerNotFound       = errors.New("session server not found")
	ErrServerExists         = errors.New("session server already exists")
	ErrMaxServersReached    = errors.New("maximum number of session servers reached")
	ErrMaxSessionsReached   = errors.New("maximum number of sessions reached")

	// 传输相关错误
	ErrInvalidPacket        = errors.New("invalid packet")
	ErrInvalidPacketHeader  = errors.New("invalid packet header")
	ErrDecryptionFailed     = errors.New("decryption failed")
	ErrEncryptionFailed     = errors.New("encryption failed")
	ErrReplayAttack         = errors.New("replay attack detected")

	// 参数错误
	ErrInvalidParameter     = errors.New("invalid parameter")
	ErrNilListener          = errors.New("listener cannot be nil")
	ErrDataTooLarge         = errors.New("data too large")

	// 状态错误
	ErrManagerNotStarted    = errors.New("session manager not started")
	ErrManagerAlreadyStarted = errors.New("session manager already started")
)
