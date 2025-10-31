package authmanager

import (
	"container/list"
	"sync"
)

// SessionKeyManager 管理会话密钥的生命周期和访问
type SessionKeyManager struct {
	keyList *list.List   // 存储会话密钥的双向链表（按添加顺序排序）
	mu      sync.RWMutex // 读写锁
}

// NewSessionKeyManager 创建一个新的会话密钥管理器
func NewSessionKeyManager() *SessionKeyManager {
	return &SessionKeyManager{
		keyList: list.New(),
	}
}

// AddSessionKey 添加或替换会话密钥
// 参数：
//   - fd：关联的文件描述符（标识连接）
//   - index：密钥索引（唯一标识）
//   - key：密钥字节数组（长度需为AuthSessionKeyLen）
// 返回：
//   - 错误（若密钥长度无效则返回ErrInvalidKeyLength）
func (m *SessionKeyManager) AddSessionKey(fd int, index int, key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 验证密钥长度是否符合要求（16字节）
	if len(key) != AuthSessionKeyLen {
		return ErrInvalidKeyLength
	}

	// 创建会话密钥对象并复制密钥内容
	skey := &SessionKey{
		Index: index,
		Fd:    fd,
	}
	copy(skey.Key[:], key)

	// 若已达最大密钥数量限制，移除最旧的密钥（链表头部元素）
	if m.keyList.Len() >= AuthSessionKeyMaxNum {
		oldest := m.keyList.Front()
		if oldest != nil {
			m.keyList.Remove(oldest)
		}
	}

	// 将新密钥添加到链表尾部（最新密钥）
	m.keyList.PushBack(skey)
	return nil
}

// GetSessionKeyByIndex 通过索引查找会话密钥
// 参数：
//   - index：密钥索引
// 返回：
//   - 找到的SessionKey（若不存在则返回nil）
func (m *SessionKeyManager) GetSessionKeyByIndex(index int) *SessionKey {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for e := m.keyList.Front(); e != nil; e = e.Next() {
		skey := e.Value.(*SessionKey)
		if skey.Index == index {
			return skey
		}
	}
	return nil
}

// GetNewSessionKey 获取最新添加的会话密钥
// 返回：
//   - 最新的SessionKey（若不存在则返回nil）
func (m *SessionKeyManager) GetNewSessionKey() *SessionKey {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 链表尾部元素为最新添加的密钥
	back := m.keyList.Back()
	if back == nil {
		return nil
	}
	return back.Value.(*SessionKey)
}

// ClearSessionKeyByFd 移除指定文件描述符关联的所有会话密钥
// 参数：
//   - fd：文件描述符（标识连接）
func (m *SessionKeyManager) ClearSessionKeyByFd(fd int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 收集所有需要删除的元素（避免边遍历边删除导致的问题）
	var toRemove []*list.Element
	for e := m.keyList.Front(); e != nil; e = e.Next() {
		skey := e.Value.(*SessionKey)
		if skey.Fd == fd {
			toRemove = append(toRemove, e)
		}
	}

	// 批量删除收集到的元素
	for _, e := range toRemove {
		m.keyList.Remove(e)
	}
}

// ClearSessionKeyBySeq 通过序列号（用作索引）移除会话密钥
// 参数：
//   - seq：序列号（与密钥索引对应）
func (m *SessionKeyManager) ClearSessionKeyBySeq(seq int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 遍历链表查找匹配索引的密钥并删除
	for e := m.keyList.Front(); e != nil; e = e.Next() {
		skey := e.Value.(*SessionKey)
		if skey.Index == int(seq) {
			m.keyList.Remove(e)
			return
		}
	}
}
