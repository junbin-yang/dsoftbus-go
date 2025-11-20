package authentication

import (
	"fmt"
	"sync"
	"time"

	"github.com/junbin-yang/dsoftbus-go/pkg/utils/crypto"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// SessionKey 会话密钥（对应C的SessionKey）
type SessionKey struct {
	Index      int32     // 密钥索引
	Key        []byte    // 会话密钥（16字节，AES-128）
	CreateTime time.Time // 创建时间
	LastUsed   time.Time // 最后使用时间
}

// SessionKeyPersistor 会话密钥持久化接口（预留）
// 外部可实现此接口以提供持久化能力
type SessionKeyPersistor interface {
	// Save 保存authId对应的所有会话密钥
	Save(authId int64, keys []*SessionKey) error

	// Load 加载authId对应的所有会话密钥
	Load(authId int64) ([]*SessionKey, error)

	// Delete 删除authId对应的所有会话密钥
	Delete(authId int64) error
}

// SessionKeyManager 会话密钥管理器
// 对应C代码中的会话密钥管理功能
// 管理HiChain派生的会话密钥
type SessionKeyManager struct {
	keys      map[int64][]*SessionKey // authId -> SessionKey列表
	persistor SessionKeyPersistor     // 持久化接口（预留）
	mu        sync.RWMutex            // 读写锁
}

// NewSessionKeyManager 创建会话密钥管理器
func NewSessionKeyManager() *SessionKeyManager {
	return &SessionKeyManager{
		keys: make(map[int64][]*SessionKey),
	}
}

// RegisterPersistor 注册持久化接口（预留）
// 外部可通过此函数注入持久化实现
func (m *SessionKeyManager) RegisterPersistor(persistor SessionKeyPersistor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persistor = persistor
	log.Infof("[SESSION_KEY] Persistor registered")
}

// SetSessionKey 设置会话密钥
// 对应C的AuthManagerSetSessionKey
//
// 参数:
//   - authId: 认证ID
//   - sessionKey: 会话密钥（16字节）
//
// 返回:
//   - int32: 密钥索引
//   - error: 错误信息
//
// 说明:
//   - 密钥索引从0开始自动递增
//   - 每次调用都会创建新的密钥记录
func (m *SessionKeyManager) SetSessionKey(authId int64, sessionKey []byte) (int32, error) {
	if len(sessionKey) != 16 {
		return -1, fmt.Errorf("session key must be 16 bytes, got %d", len(sessionKey))
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取或创建authId对应的密钥列表
	keyList, exists := m.keys[authId]
	if !exists {
		keyList = make([]*SessionKey, 0)
	}

	// 计算新密钥的索引（自动递增）
	index := int32(0)
	if len(keyList) > 0 {
		index = keyList[len(keyList)-1].Index + 1
	}

	// 创建新密钥
	keyCopy := make([]byte, 16)
	copy(keyCopy, sessionKey)

	newKey := &SessionKey{
		Index:      index,
		Key:        keyCopy,
		CreateTime: time.Now(),
		LastUsed:   time.Now(),
	}

	// 添加到列表
	keyList = append(keyList, newKey)
	m.keys[authId] = keyList

	log.Infof("[SESSION_KEY] Session key set: authId=%d, index=%d, keyLen=%d",
		authId, index, len(sessionKey))

	// 如果注册了持久化接口，调用保存
	if m.persistor != nil {
		if err := m.persistor.Save(authId, keyList); err != nil {
			log.Warnf("[SESSION_KEY] Failed to persist session key: %v", err)
			// 持久化失败不影响内存操作
		}
	}

	return index, nil
}

// GetSessionKey 获取指定索引的会话密钥
// 对应C的AuthManagerGetSessionKey
//
// 参数:
//   - authId: 认证ID
//   - index: 密钥索引
//
// 返回:
//   - *SessionKey: 会话密钥
//   - error: 错误信息（密钥不存在时）
func (m *SessionKeyManager) GetSessionKey(authId int64, index int32) (*SessionKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keyList, exists := m.keys[authId]
	if !exists {
		return nil, fmt.Errorf("no session key found for authId=%d", authId)
	}

	// 查找指定索引的密钥
	for _, key := range keyList {
		if key.Index == index {
			// 更新最后使用时间
			key.LastUsed = time.Now()
			log.Debugf("[SESSION_KEY] Session key retrieved: authId=%d, index=%d", authId, index)
			return key, nil
		}
	}

	return nil, fmt.Errorf("session key not found: authId=%d, index=%d", authId, index)
}

// GetLatestSessionKey 获取最新的会话密钥
// 用于需要使用最新密钥的场景
//
// 参数:
//   - authId: 认证ID
//
// 返回:
//   - *SessionKey: 最新的会话密钥
//   - error: 错误信息（没有密钥时）
func (m *SessionKeyManager) GetLatestSessionKey(authId int64) (*SessionKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keyList, exists := m.keys[authId]
	if !exists || len(keyList) == 0 {
		return nil, fmt.Errorf("no session key found for authId=%d", authId)
	}

	// 返回最后一个密钥（索引最大）
	latestKey := keyList[len(keyList)-1]
	latestKey.LastUsed = time.Now()

	log.Debugf("[SESSION_KEY] Latest session key retrieved: authId=%d, index=%d", authId, latestKey.Index)
	return latestKey, nil
}

// RemoveSessionKey 删除指定索引的会话密钥
//
// 参数:
//   - authId: 认证ID
//   - index: 密钥索引
func (m *SessionKeyManager) RemoveSessionKey(authId int64, index int32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	keyList, exists := m.keys[authId]
	if !exists {
		return
	}

	// 查找并删除指定索引的密钥
	for i, key := range keyList {
		if key.Index == index {
			keyList = append(keyList[:i], keyList[i+1:]...)
			m.keys[authId] = keyList
			log.Infof("[SESSION_KEY] Session key removed: authId=%d, index=%d", authId, index)

			// 持久化
			if m.persistor != nil {
				if err := m.persistor.Save(authId, keyList); err != nil {
					log.Warnf("[SESSION_KEY] Failed to persist after removal: %v", err)
				}
			}
			return
		}
	}
}

// RemoveAllSessionKeys 删除authId对应的所有会话密钥
//
// 参数:
//   - authId: 认证ID
func (m *SessionKeyManager) RemoveAllSessionKeys(authId int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.keys, authId)
	log.Infof("[SESSION_KEY] All session keys removed: authId=%d", authId)

	// 持久化删除
	if m.persistor != nil {
		if err := m.persistor.Delete(authId); err != nil {
			log.Warnf("[SESSION_KEY] Failed to persist deletion: %v", err)
		}
	}
}

// GetSessionKeyCount 获取authId对应的密钥数量
//
// 参数:
//   - authId: 认证ID
//
// 返回:
//   - int: 密钥数量
func (m *SessionKeyManager) GetSessionKeyCount(authId int64) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keyList, exists := m.keys[authId]
	if !exists {
		return 0
	}

	return len(keyList)
}

// ============================================================================
// 加密/解密接口（预留，当前返回未实现错误）
// ============================================================================

var (
	ErrEncryptionNotImplemented = fmt.Errorf("encryption not implemented")
	ErrDecryptionNotImplemented = fmt.Errorf("decryption not implemented")
)

// Encrypt 使用会话密钥加密数据
// 对应C的AuthDeviceEncrypt
//
// 当前实现: 返回未实现错误（预留接口）
// 未来实现: 使用AES-128-GCM加密
//
// 参数:
//   - authId: 认证ID
//   - plaintext: 明文数据
//
// 返回:
//   - []byte: 密文数据
//   - error: 错误信息
func (m *SessionKeyManager) Encrypt(authId int64, plaintext []byte) ([]byte, error) {
	// 1. 获取最新的会话密钥
	key, err := m.GetLatestSessionKey(authId)
	if err != nil {
		return nil, fmt.Errorf("failed to get session key: %w", err)
	}

	// 2. 使用AES-GCM加密（自动生成IV）
	encrypted, err := crypto.EncryptAESGCM(key.Key, plaintext)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	// 3. 添加密钥索引前缀: [索引(4字节)] + [IV+密文+Tag]
	result := make([]byte, 4+len(encrypted))
	result[0] = byte(key.Index >> 24)
	result[1] = byte(key.Index >> 16)
	result[2] = byte(key.Index >> 8)
	result[3] = byte(key.Index)
	copy(result[4:], encrypted)

	return result, nil
}

// Decrypt 使用会话密钥解密数据
// 对应C的AuthDeviceDecrypt
//
// 当前实现: 返回未实现错误（预留接口）
// 未来实现: 使用AES-128-GCM解密
//
// 参数:
//   - authId: 认证ID
//   - ciphertext: 密文数据
//
// 返回:
//   - []byte: 明文数据
//   - error: 错误信息
func (m *SessionKeyManager) Decrypt(authId int64, ciphertext []byte) ([]byte, error) {
	// 1. 解析密钥索引: [索引(4字节)] + [IV+密文+Tag]
	if len(ciphertext) < 4 {
		return nil, fmt.Errorf("ciphertext too short")
	}

	keyIndex := int32(ciphertext[0])<<24 | int32(ciphertext[1])<<16 | int32(ciphertext[2])<<8 | int32(ciphertext[3])
	encryptedData := ciphertext[4:]

	// 2. 根据索引获取会话密钥
	key, err := m.GetSessionKey(authId, keyIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to get session key: %w", err)
	}

	// 3. 使用AES-GCM解密
	plaintext, err := crypto.DecryptAESGCM(key.Key, encryptedData)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// ============================================================================
// 持久化相关（预留）
// ============================================================================

// LoadFromPersistor 从持久化存储加载所有会话密钥（预留）
//
// 参数:
//   - authId: 认证ID
//
// 返回:
//   - error: 错误信息
func (m *SessionKeyManager) LoadFromPersistor(authId int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.persistor == nil {
		return fmt.Errorf("persistor not registered")
	}

	keys, err := m.persistor.Load(authId)
	if err != nil {
		return fmt.Errorf("failed to load session keys: %w", err)
	}

	m.keys[authId] = keys
	log.Infof("[SESSION_KEY] Loaded %d session keys from persistor: authId=%d", len(keys), authId)

	return nil
}
