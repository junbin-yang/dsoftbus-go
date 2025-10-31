package authmanager

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
)

// GenerateRandomIV 生成AES-GCM算法所需的随机初始化向量(IV)
func GenerateRandomIV() ([]byte, error) {
	iv := make([]byte, MessageGcmNonceLen)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	return iv, nil
}

// EncryptTransData 使用AES-GCM算法和会话密钥加密传输数据
// 参数：
//   - key：AES密钥（长度需为16/24/32字节）
//   - iv：初始化向量（12字节）
//   - plaintext：待加密的明文数据
// 返回：
//   - 加密后的数据（格式：IV(12字节) + 密文 + 认证标签(16字节)）
//   - 错误信息（若加密失败）
func EncryptTransData(key []byte, iv []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Output format: IV(12) + ciphertext + tag(16)
	// 加密并生成认证标签（GCM模式会自动附加16字节标签）
	ciphertext := aesgcm.Seal(nil, iv, plaintext, nil)

	// 拼接IV和加密数据（IV在前，密文+标签在后）
	result := make([]byte, len(iv)+len(ciphertext))
	copy(result, iv)
	copy(result[len(iv):], ciphertext)

	return result, nil
}

// DecryptTransData 使用AES-GCM算法和会话密钥解密传输数据
// 参数：
//   - key：AES密钥（与加密时使用的密钥一致）
//   - cipherData：加密后的数据（格式：IV + 密文 + 标签）
// 返回：
//   - 解密后的明文数据
//   - 错误信息（若解密失败或数据无效）
func DecryptTransData(key []byte, cipherData []byte) ([]byte, error) {
	// 验证数据长度是否满足最小要求（IV长度+标签长度）
	if len(cipherData) < MessageGcmNonceLen+MessageGcmMacLen {
		return nil, fmt.Errorf("密文长度过短")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// 从加密数据中提取IV和密文（含标签）
	iv := cipherData[:MessageGcmNonceLen]
	ciphertext := cipherData[MessageGcmNonceLen:]

	// 解密并验证标签（验证数据完整性和真实性）
	plaintext, err := aesgcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
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
	// 生成随机IV
	randomIV, err := GenerateRandomIV()
	if err != nil {
		return nil, err
	}

	// 组合随机IV和序列号（用序列号覆盖IV的前8字节，增强唯一性）
	iv := make([]byte, MessageGcmNonceLen)
	copy(iv, randomIV)
	binary.LittleEndian.PutUint64(iv[:8], uint64(seqNum))

	// 使用会话密钥加密数据
	encrypted, err := EncryptTransData(skey.Key[:], iv, dataIn)
	if err != nil {
		return nil, err
	}

	// 在加密数据前添加4字节的密钥索引（用于接收方查找对应密钥）
	result := make([]byte, 4+len(encrypted))
	binary.LittleEndian.PutUint32(result[:4], uint32(skey.Index))
	copy(result[4:], encrypted)

	return result, nil
}
