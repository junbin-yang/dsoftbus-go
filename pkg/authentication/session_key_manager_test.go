package authentication

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// MockSessionKeyPersistor Mock持久化接口（用于测试）
type MockSessionKeyPersistor struct {
	storage map[int64][]*SessionKey
	mu      sync.Mutex
	err     error
}

func NewMockSessionKeyPersistor() *MockSessionKeyPersistor {
	return &MockSessionKeyPersistor{
		storage: make(map[int64][]*SessionKey),
	}
}

func (m *MockSessionKeyPersistor) Save(authId int64, keys []*SessionKey) error {
	if m.err != nil {
		return m.err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Deep copy keys
	copiedKeys := make([]*SessionKey, len(keys))
	for i, key := range keys {
		keyCopy := make([]byte, len(key.Key))
		copy(keyCopy, key.Key)
		copiedKeys[i] = &SessionKey{
			Index:      key.Index,
			Key:        keyCopy,
			CreateTime: key.CreateTime,
			LastUsed:   key.LastUsed,
		}
	}

	m.storage[authId] = copiedKeys
	return nil
}

func (m *MockSessionKeyPersistor) Load(authId int64) ([]*SessionKey, error) {
	if m.err != nil {
		return nil, m.err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	keys, exists := m.storage[authId]
	if !exists {
		return nil, fmt.Errorf("no keys found for authId=%d", authId)
	}

	return keys, nil
}

func (m *MockSessionKeyPersistor) Delete(authId int64) error {
	if m.err != nil {
		return m.err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.storage, authId)
	return nil
}

// 测试创建SessionKeyManager
func TestNewSessionKeyManager(t *testing.T) {
	manager := NewSessionKeyManager()
	if manager == nil {
		t.Fatal("NewSessionKeyManager returned nil")
	}

	if manager.keys == nil {
		t.Error("SessionKeyManager.keys should not be nil")
	}

	t.Log("NewSessionKeyManager test passed")
}

// 测试设置和获取会话密钥
func TestSessionKeyManager_SetAndGet(t *testing.T) {
	manager := NewSessionKeyManager()

	// 测试设置第一个密钥
	sessionKey1 := []byte("1234567890abcdef") // 16字节
	authId := int64(1001)

	index1, err := manager.SetSessionKey(authId, sessionKey1)
	if err != nil {
		t.Fatalf("SetSessionKey failed: %v", err)
	}
	if index1 != 0 {
		t.Errorf("First key index should be 0, got %d", index1)
	}
	t.Logf("Set session key: authId=%d, index=%d", authId, index1)

	// 测试获取密钥
	key, err := manager.GetSessionKey(authId, index1)
	if err != nil {
		t.Fatalf("GetSessionKey failed: %v", err)
	}
	if key.Index != index1 {
		t.Errorf("Key index mismatch: expected %d, got %d", index1, key.Index)
	}
	if string(key.Key) != string(sessionKey1) {
		t.Errorf("Key data mismatch: expected %s, got %s", sessionKey1, key.Key)
	}
	t.Logf("Get session key: authId=%d, index=%d, keyLen=%d", authId, key.Index, len(key.Key))

	// 测试设置第二个密钥（索引应该递增）
	sessionKey2 := []byte("fedcba0987654321")
	index2, err := manager.SetSessionKey(authId, sessionKey2)
	if err != nil {
		t.Fatalf("SetSessionKey failed: %v", err)
	}
	if index2 != 1 {
		t.Errorf("Second key index should be 1, got %d", index2)
	}
	t.Logf("Set session key: authId=%d, index=%d", authId, index2)

	// 测试获取第二个密钥
	key2, err := manager.GetSessionKey(authId, index2)
	if err != nil {
		t.Fatalf("GetSessionKey failed: %v", err)
	}
	if string(key2.Key) != string(sessionKey2) {
		t.Errorf("Key data mismatch: expected %s, got %s", sessionKey2, key2.Key)
	}

	// 测试密钥数量
	count := manager.GetSessionKeyCount(authId)
	if count != 2 {
		t.Errorf("Key count should be 2, got %d", count)
	}
	t.Logf("Session key count: %d", count)

	t.Log("SetSessionKey and GetSessionKey test passed")
}

// 测试获取最新密钥
func TestSessionKeyManager_GetLatest(t *testing.T) {
	manager := NewSessionKeyManager()
	authId := int64(2001)

	// 设置多个密钥
	keys := [][]byte{
		[]byte("key0000000000000"), // 16字节
		[]byte("key1111111111111"), // 16字节
		[]byte("key2222222222222"), // 16字节
	}

	for _, key := range keys {
		_, err := manager.SetSessionKey(authId, key)
		if err != nil {
			t.Fatalf("SetSessionKey failed: %v", err)
		}
	}

	// 获取最新密钥（应该是最后一个）
	latestKey, err := manager.GetLatestSessionKey(authId)
	if err != nil {
		t.Fatalf("GetLatestSessionKey failed: %v", err)
	}
	if latestKey.Index != 2 {
		t.Errorf("Latest key index should be 2, got %d", latestKey.Index)
	}
	if string(latestKey.Key) != string(keys[2]) {
		t.Errorf("Latest key data mismatch: expected %s, got %s", keys[2], latestKey.Key)
	}
	t.Logf("Latest session key: authId=%d, index=%d", authId, latestKey.Index)

	t.Log("GetLatestSessionKey test passed")
}

// 测试删除密钥
func TestSessionKeyManager_Remove(t *testing.T) {
	manager := NewSessionKeyManager()
	authId := int64(3001)

	// 设置3个密钥
	for i := 0; i < 3; i++ {
		key := []byte(fmt.Sprintf("key%013d", i)) // 16字节 (key + 13位数字)
		_, err := manager.SetSessionKey(authId, key)
		if err != nil {
			t.Fatalf("SetSessionKey failed: %v", err)
		}
	}

	count := manager.GetSessionKeyCount(authId)
	if count != 3 {
		t.Errorf("Initial count should be 3, got %d", count)
	}

	// 删除索引为1的密钥
	manager.RemoveSessionKey(authId, 1)

	count = manager.GetSessionKeyCount(authId)
	if count != 2 {
		t.Errorf("After removal, count should be 2, got %d", count)
	}

	// 尝试获取已删除的密钥（应该失败）
	_, err := manager.GetSessionKey(authId, 1)
	if err == nil {
		t.Error("Should fail when getting removed key")
	}
	t.Logf("GetSessionKey after removal error (expected): %v", err)

	// 删除所有密钥
	manager.RemoveAllSessionKeys(authId)

	count = manager.GetSessionKeyCount(authId)
	if count != 0 {
		t.Errorf("After removing all, count should be 0, got %d", count)
	}

	t.Log("RemoveSessionKey test passed")
}

// 测试无效密钥长度
func TestSessionKeyManager_InvalidKeyLength(t *testing.T) {
	manager := NewSessionKeyManager()
	authId := int64(4001)

	// 测试短密钥
	shortKey := []byte("short")
	_, err := manager.SetSessionKey(authId, shortKey)
	if err == nil {
		t.Error("Should fail with short key")
	}
	t.Logf("Short key error (expected): %v", err)

	// 测试长密钥
	longKey := []byte("this is a very long key")
	_, err = manager.SetSessionKey(authId, longKey)
	if err == nil {
		t.Error("Should fail with long key")
	}
	t.Logf("Long key error (expected): %v", err)

	// 测试空密钥
	emptyKey := []byte{}
	_, err = manager.SetSessionKey(authId, emptyKey)
	if err == nil {
		t.Error("Should fail with empty key")
	}
	t.Logf("Empty key error (expected): %v", err)

	t.Log("Invalid key length test passed")
}

// 测试获取不存在的密钥
func TestSessionKeyManager_GetNonexistent(t *testing.T) {
	manager := NewSessionKeyManager()

	// 测试不存在的authId
	_, err := manager.GetSessionKey(9999, 0)
	if err == nil {
		t.Error("Should fail when getting key for nonexistent authId")
	}
	t.Logf("Nonexistent authId error (expected): %v", err)

	// 设置一个密钥
	authId := int64(5001)
	key := []byte("1234567890abcdef")
	manager.SetSessionKey(authId, key)

	// 测试不存在的索引
	_, err = manager.GetSessionKey(authId, 99)
	if err == nil {
		t.Error("Should fail when getting key with nonexistent index")
	}
	t.Logf("Nonexistent index error (expected): %v", err)

	// 测试GetLatestSessionKey for nonexistent authId
	_, err = manager.GetLatestSessionKey(9999)
	if err == nil {
		t.Error("Should fail when getting latest key for nonexistent authId")
	}
	t.Logf("GetLatestSessionKey error (expected): %v", err)

	t.Log("Get nonexistent key test passed")
}

// 测试加密/解密（应该返回未实现错误）
func TestSessionKeyManager_EncryptDecrypt(t *testing.T) {
	manager := NewSessionKeyManager()
	authId := int64(6001)

	// 设置密钥
	key := []byte("1234567890abcdef")
	manager.SetSessionKey(authId, key)

	// 测试加密（应该失败）
	plaintext := []byte("Hello, World!")
	_, err := manager.Encrypt(authId, plaintext)
	if err != ErrEncryptionNotImplemented {
		t.Errorf("Expected ErrEncryptionNotImplemented, got %v", err)
	}
	t.Logf("Encrypt error (expected): %v", err)

	// 测试解密（应该失败）
	ciphertext := []byte("encrypted data")
	_, err = manager.Decrypt(authId, ciphertext)
	if err != ErrDecryptionNotImplemented {
		t.Errorf("Expected ErrDecryptionNotImplemented, got %v", err)
	}
	t.Logf("Decrypt error (expected): %v", err)

	t.Log("Encrypt/Decrypt not implemented test passed")
}

// 测试持久化接口
func TestSessionKeyManager_Persistor(t *testing.T) {
	manager := NewSessionKeyManager()
	persistor := NewMockSessionKeyPersistor()

	// 注册持久化接口
	manager.RegisterPersistor(persistor)

	authId := int64(7001)
	key := []byte("1234567890abcdef")

	// 设置密钥（应该自动持久化）
	index, err := manager.SetSessionKey(authId, key)
	if err != nil {
		t.Fatalf("SetSessionKey failed: %v", err)
	}
	t.Logf("Set session key with persistor: authId=%d, index=%d", authId, index)

	// 验证持久化存储
	persistor.mu.Lock()
	persistedKeys, exists := persistor.storage[authId]
	persistor.mu.Unlock()

	if !exists {
		t.Error("Key should be persisted")
	}
	if len(persistedKeys) != 1 {
		t.Errorf("Persisted key count should be 1, got %d", len(persistedKeys))
	}
	if string(persistedKeys[0].Key) != string(key) {
		t.Error("Persisted key data mismatch")
	}
	t.Logf("Verified persisted keys: count=%d", len(persistedKeys))

	// 测试删除（应该删除持久化存储）
	manager.RemoveAllSessionKeys(authId)

	persistor.mu.Lock()
	_, exists = persistor.storage[authId]
	persistor.mu.Unlock()

	if exists {
		t.Error("Key should be removed from persistor")
	}

	t.Log("Persistor test passed")
}

// 测试从持久化加载
func TestSessionKeyManager_LoadFromPersistor(t *testing.T) {
	manager := NewSessionKeyManager()
	persistor := NewMockSessionKeyPersistor()
	manager.RegisterPersistor(persistor)

	authId := int64(8001)
	key := []byte("1234567890abcdef")

	// 直接写入持久化存储
	persistor.storage[authId] = []*SessionKey{
		{
			Index:      0,
			Key:        key,
			CreateTime: time.Now(),
			LastUsed:   time.Now(),
		},
	}

	// 从持久化加载
	err := manager.LoadFromPersistor(authId)
	if err != nil {
		t.Fatalf("LoadFromPersistor failed: %v", err)
	}

	// 验证加载的密钥
	loadedKey, err := manager.GetSessionKey(authId, 0)
	if err != nil {
		t.Fatalf("GetSessionKey after load failed: %v", err)
	}
	if string(loadedKey.Key) != string(key) {
		t.Error("Loaded key data mismatch")
	}
	t.Logf("Loaded session key: authId=%d, index=%d", authId, loadedKey.Index)

	t.Log("LoadFromPersistor test passed")
}

// 测试持久化错误处理
func TestSessionKeyManager_PersistorError(t *testing.T) {
	manager := NewSessionKeyManager()
	persistor := NewMockSessionKeyPersistor()
	persistor.err = fmt.Errorf("mock persistor error")
	manager.RegisterPersistor(persistor)

	authId := int64(9001)
	key := []byte("1234567890abcdef")

	// 设置密钥（持久化失败不应影响内存操作）
	index, err := manager.SetSessionKey(authId, key)
	if err != nil {
		t.Fatalf("SetSessionKey should succeed even if persistor fails: %v", err)
	}
	t.Logf("SetSessionKey succeeded despite persistor error: authId=%d, index=%d", authId, index)

	// 验证内存中的密钥仍然存在
	retrievedKey, err := manager.GetSessionKey(authId, index)
	if err != nil {
		t.Fatalf("GetSessionKey failed: %v", err)
	}
	if string(retrievedKey.Key) != string(key) {
		t.Error("Key data mismatch after persistor error")
	}

	t.Log("Persistor error handling test passed")
}

// 测试并发安全
func TestSessionKeyManager_Concurrency(t *testing.T) {
	manager := NewSessionKeyManager()
	authId := int64(10001)

	var wg sync.WaitGroup
	concurrency := 100
	errors := make(chan error, concurrency*2)

	// 并发设置密钥
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := []byte(fmt.Sprintf("key%013d", idx)) // 16字节
			_, err := manager.SetSessionKey(authId, key)
			if err != nil {
				errors <- fmt.Errorf("SetSessionKey failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// 验证所有密钥都设置成功
	count := manager.GetSessionKeyCount(authId)
	if count != concurrency {
		t.Errorf("Key count should be %d, got %d", concurrency, count)
	}
	t.Logf("Concurrency test: %d keys set successfully", count)

	// 并发读取密钥
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := manager.GetSessionKey(authId, int32(idx))
			if err != nil {
				errors <- fmt.Errorf("GetSessionKey failed: %v", err)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 检查是否有错误
	for err := range errors {
		t.Errorf("Concurrency test error: %v", err)
	}

	t.Logf("Concurrency test completed: %d concurrent operations", concurrency*2)
	t.Log("Concurrency test passed")
}

// 测试多个authId
func TestSessionKeyManager_MultipleAuthIds(t *testing.T) {
	manager := NewSessionKeyManager()

	// 为不同的authId设置密钥
	authIds := []int64{1001, 1002, 1003}
	for _, authId := range authIds {
		key := []byte(fmt.Sprintf("key%013d", authId)) // 16字节
		_, err := manager.SetSessionKey(authId, key)
		if err != nil {
			t.Fatalf("SetSessionKey failed for authId=%d: %v", authId, err)
		}
	}

	// 验证每个authId的密钥独立
	for _, authId := range authIds {
		count := manager.GetSessionKeyCount(authId)
		if count != 1 {
			t.Errorf("AuthId=%d should have 1 key, got %d", authId, count)
		}

		key, err := manager.GetSessionKey(authId, 0)
		if err != nil {
			t.Fatalf("GetSessionKey failed for authId=%d: %v", authId, err)
		}

		expectedKey := fmt.Sprintf("key%013d", authId) // 16字节
		if string(key.Key) != expectedKey {
			t.Errorf("AuthId=%d key mismatch: expected %s, got %s", authId, expectedKey, key.Key)
		}
	}

	t.Logf("Multiple authIds test passed: %d authIds", len(authIds))
	t.Log("Multiple authIds test passed")
}
