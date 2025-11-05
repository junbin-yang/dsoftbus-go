package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

const (
	// GCM模式常量
	GcmNonceLen = 12                      // GCM模式的Nonce(IV)长度
	GcmTagLen   = 16                      // GCM模式的MAC标签长度
	OverheadLen = GcmNonceLen + GcmTagLen // 总开销（IV + TAG）
)

// GenerateRandomBytes 生成指定长度的随机字节
// 参数：
//   - length：要生成的随机字节数
// 返回：
//   - 随机字节数组
//   - 错误信息
func GenerateRandomBytes(length int) ([]byte, error) {
	bytes := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return nil, err
	}
	return bytes, nil
}

// GenerateRandomIV 生成AES-GCM算法所需的随机初始化向量(IV)
// 返回：
//   - IV字节数组（12字节）
//   - 错误信息
func GenerateRandomIV() ([]byte, error) {
	return GenerateRandomBytes(GcmNonceLen)
}

// EncryptAESGCM 使用AES-GCM算法加密数据
// 参数：
//   - key：AES密钥（长度需为16/24/32字节，对应128/192/256位）
//   - plaintext：待加密的明文数据
// 返回：
//   - 加密后的数据（格式：IV(12字节) + 密文 + 认证标签(16字节)）
//   - 错误信息（若加密失败）
//
// 注意：此函数会自动生成随机IV
func EncryptAESGCM(key []byte, plaintext []byte) ([]byte, error) {
	// 生成随机IV
	iv, err := GenerateRandomIV()
	if err != nil {
		return nil, fmt.Errorf("生成IV失败: %w", err)
	}

	// 创建AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建cipher失败: %w", err)
	}

	// 创建GCM模式
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建GCM失败: %w", err)
	}

	// 加密并生成认证标签（GCM模式会自动附加16字节标签）
	ciphertext := aesgcm.Seal(nil, iv, plaintext, nil)

	// 拼接IV和加密数据（IV在前，密文+标签在后）
	result := make([]byte, len(iv)+len(ciphertext))
	copy(result, iv)
	copy(result[len(iv):], ciphertext)

	return result, nil
}

// DecryptAESGCM 使用AES-GCM算法解密数据
// 参数：
//   - key：AES密钥（与加密时使用的密钥一致）
//   - cipherData：加密后的数据（格式：IV + 密文 + 标签）
// 返回：
//   - 解密后的明文数据
//   - 错误信息（若解密失败或数据无效）
func DecryptAESGCM(key []byte, cipherData []byte) ([]byte, error) {
	// 验证数据长度是否满足最小要求（IV长度+标签长度）
	if len(cipherData) < OverheadLen {
		return nil, fmt.Errorf("密文长度过短，至少需要%d字节", OverheadLen)
	}

	// 创建AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建cipher失败: %w", err)
	}

	// 创建GCM模式
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建GCM失败: %w", err)
	}

	// 从加密数据中提取IV和密文（含标签）
	iv := cipherData[:GcmNonceLen]
	ciphertext := cipherData[GcmNonceLen:]

	// 解密并验证标签（验证数据完整性和真实性）
	plaintext, err := aesgcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("解密失败（可能是密钥错误或数据被篡改）: %w", err)
	}

	return plaintext, nil
}
