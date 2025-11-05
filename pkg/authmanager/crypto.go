package authmanager

import (
	"encoding/binary"

	"github.com/junbin-yang/dsoftbus-go/pkg/utils/crypto"
)

// 导出utils/crypto包中的常量，保持向后兼容
const (
	MessageGcmNonceLen = crypto.GcmNonceLen // GCM模式的Nonce(IV)长度 = 12
	MessageGcmMacLen   = crypto.GcmTagLen   // GCM模式的MAC标签长度 = 16
)

// GenerateRandomIV 生成AES-GCM算法所需的随机初始化向量(IV)
// 注意：此函数现在是utils/crypto包的wrapper，保持API兼容性
func GenerateRandomIV() ([]byte, error) {
	return crypto.GenerateRandomIV()
}

// EncryptTransData 使用AES-GCM算法和会话密钥加密传输数据
// 注意：此函数签名保持兼容，但内部调用utils/crypto
// 参数：
//   - key：AES密钥（长度需为16/24/32字节）
//   - iv：初始化向量（12字节）- 注意：此参数为兼容旧API保留，实际会被忽略
//   - plaintext：待加密的明文数据
// 返回：
//   - 加密后的数据（格式：IV(12字节) + 密文 + 认证标签(16字节)）
//   - 错误信息（若加密失败）
func EncryptTransData(key []byte, iv []byte, plaintext []byte) ([]byte, error) {
	// 注意：为了与utils/crypto统一，这里忽略传入的iv参数，使用随机IV
	// 这与C版本的实现一致（每次加密都生成新的随机IV）
	return crypto.EncryptAESGCM(key, plaintext)
}

// DecryptTransData 使用AES-GCM算法和会话密钥解密传输数据
// 注意：此函数现在是utils/crypto包的wrapper，保持API兼容性
// 参数：
//   - key：AES密钥（与加密时使用的密钥一致）
//   - cipherData：加密后的数据（格式：IV + 密文 + 标签）
// 返回：
//   - 解密后的明文数据
//   - 错误信息（若解密失败或数据无效）
func DecryptTransData(key []byte, cipherData []byte) ([]byte, error) {
	return crypto.DecryptAESGCM(key, cipherData)
}

// GetEncryptTransData 使用会话密钥加密数据，并在头部添加密钥索引
// 功能：整合IV生成、加密、索引添加的完整流程，用于实际传输
// 参数：
//   - seqNum：序列号（用于增强IV的唯一性）
//   - dataIn：待加密的原始数据
//   - skey：会话密钥（含密钥内容和索引）
// 返回：
//   - 最终传输数据（格式：索引(4字节) + IV(12字节) + 密文 + 标签(16字节)）
//   - 错误信息（若过程失败）
func GetEncryptTransData(seqNum int64, dataIn []byte, skey *SessionKey) ([]byte, error) {
	// 使用统一的加密函数
	encrypted, err := crypto.EncryptAESGCM(skey.Key[:], dataIn)
	if err != nil {
		return nil, err
	}

	// 在加密数据前添加4字节的密钥索引（用于接收方查找对应密钥）
	result := make([]byte, 4+len(encrypted))
	binary.LittleEndian.PutUint32(result[:4], uint32(skey.Index))
	copy(result[4:], encrypted)

	return result, nil
}
