package authmanager

import (
	"encoding/binary"
	"testing"
)

// TestGenerateRandomIV 测试随机IV生成功能
func TestGenerateRandomIV(t *testing.T) {
	iv, err := GenerateRandomIV()
	if err != nil {
		t.Fatalf("生成随机IV失败: %v", err)
	}

	// 验证IV长度是否正确
	if len(iv) != MessageGcmNonceLen {
		t.Errorf("IV长度不正确，期望%d，实际%d", MessageGcmNonceLen, len(iv))
	}

	// 验证两次生成的IV是否不同（随机性测试）
	iv2, _ := GenerateRandomIV()
	if string(iv) == string(iv2) {
		t.Error("两次生成的IV相同，随机性存在问题")
	}
}

// TestEncryptDecryptTransData 测试加密解密基本功能
func TestEncryptDecryptTransData(t *testing.T) {
	// 准备测试数据
	key := make([]byte, AuthSessionKeyLen) // 16字节密钥
	for i := 0; i < AuthSessionKeyLen; i++ {
		key[i] = byte(i) // 填充测试密钥
	}
	plaintext := []byte("测试AES-GCM加密解密功能")
	iv, _ := GenerateRandomIV()

	// 加密数据
	cipherData, err := EncryptTransData(key, iv, plaintext)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}

	// 验证加密后的数据长度（IV + 密文 + 标签）
	expectedMinLen := MessageGcmNonceLen + len(plaintext) + MessageGcmMacLen
	if len(cipherData) < expectedMinLen {
		t.Errorf("加密后数据长度异常，期望至少%d，实际%d", expectedMinLen, len(cipherData))
	}

	// 解密数据
	decrypted, err := DecryptTransData(key, cipherData)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}

	// 验证解密结果是否与原文一致
	if string(decrypted) != string(plaintext) {
		t.Errorf("解密结果不匹配，期望'%s'，实际'%s'", plaintext, decrypted)
	}
}

// TestDecryptWithWrongKey 测试使用错误密钥解密的情况
func TestDecryptWithWrongKey(t *testing.T) {
	// 准备测试数据
	correctKey := make([]byte, AuthSessionKeyLen)
	wrongKey := make([]byte, AuthSessionKeyLen)
	for i := 0; i < AuthSessionKeyLen; i++ {
		correctKey[i] = byte(i)
		wrongKey[i] = byte(i + 1) // 错误的密钥
	}
	plaintext := []byte("测试错误密钥解密")
	iv, _ := GenerateRandomIV()

	// 用正确密钥加密
	cipherData, _ := EncryptTransData(correctKey, iv, plaintext)

	// 用错误密钥解密（应失败）
	_, err := DecryptTransData(wrongKey, cipherData)
	if err == nil {
		t.Error("使用错误密钥解密应该失败，但未返回错误")
	}
}

// TestGetEncryptTransData 测试完整的加密传输数据生成与解密流程
func TestGetEncryptTransData(t *testing.T) {
	// 准备测试数据
	seqNum := int64(123456) // 测试用序列号
	plaintext := []byte("测试完整的加密传输流程")

	// 准备会话密钥
	var skey SessionKey
	for i := 0; i < AuthSessionKeyLen; i++ {
		skey.Key[i] = byte(i * 2) // 填充测试密钥
	}
	skey.Index = 1 // 密钥索引

	// 生成加密传输数据
	encryptedData, err := GetEncryptTransData(seqNum, plaintext, &skey)
	if err != nil {
		t.Fatalf("生成加密传输数据失败: %v", err)
	}

	// 验证数据长度（索引4字节 + IV12字节 + 密文 + 标签16字节）
	expectedMinLen := 4 + MessageGcmNonceLen + len(plaintext) + MessageGcmMacLen
	if len(encryptedData) < expectedMinLen {
		t.Errorf("加密传输数据长度异常，期望至少%d，实际%d", expectedMinLen, len(encryptedData))
	}

	// 解析密钥索引（前4字节）
	index := binary.LittleEndian.Uint32(encryptedData[:4])
	if int(index) != skey.Index {
		t.Errorf("密钥索引不匹配，期望%d，实际%d", skey.Index, index)
	}

	// 提取加密部分并解密
	cipherData := encryptedData[4:]
	decrypted, err := DecryptTransData(skey.Key[:], cipherData)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}

	// 验证解密结果
	if string(decrypted) != string(plaintext) {
		t.Errorf("解密结果不匹配，期望'%s'，实际'%s'", plaintext, decrypted)
	}
}

// TestEdgeCases 测试边界情况
func TestEdgeCases(t *testing.T) {
	// 测试空数据加密解密
	key := make([]byte, AuthSessionKeyLen)
	iv, _ := GenerateRandomIV()
	emptyData := []byte("")

	cipherData, err := EncryptTransData(key, iv, emptyData)
	if err != nil {
		t.Errorf("空数据加密失败: %v", err)
	}

	decrypted, err := DecryptTransData(key, cipherData)
	if err != nil {
		t.Errorf("空数据解密失败: %v", err)
	}
	if len(decrypted) != 0 {
		t.Error("空数据解密结果不应非空")
	}

	// 测试过短的密文解密（应失败）
	shortData := make([]byte, MessageGcmNonceLen+MessageGcmMacLen-1)
	_, err = DecryptTransData(key, shortData)
	if err == nil {
		t.Error("过短的密文解密应该失败，但未返回错误")
	}
}
