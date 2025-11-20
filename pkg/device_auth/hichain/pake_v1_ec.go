package hichain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// PAKE V1 EC-SPEKE 协议实现（基于 X25519）
// 参考 HarmonyOS pake_v1_protocol 实现

const (
	HiChainSpekeBaseInfo       = "hichain_speke_base_info"
	HiChainSpekeSessionKeyInfo = "hichain_speke_sessionkey_info"
)

// computeX25519BasePoint 计算 SPEKE 基点
// base = Elligator(HKDF(PSK, salt, info))
func computeX25519BasePoint(psk []byte, salt []byte) ([]byte, error) {
	if len(psk) == 0 {
		return nil, fmt.Errorf("PSK不能为空")
	}

	// 使用HKDF派生secret
	hkdfReader := hkdf.New(sha256.New, psk, salt, []byte(HiChainSpekeBaseInfo))
	secret := make([]byte, 32)
	_, err := hkdfReader.Read(secret)
	if err != nil {
		return nil, fmt.Errorf("HKDF派生secret失败: %w", err)
	}

	// 应用 Elligator 算法
	base, err := elligatorCurve25519(secret)
	if err != nil {
		return nil, fmt.Errorf("Elligator算法失败: %w", err)
	}

	return base, nil
}

// GenerateX25519KeyPair 生成X25519密钥对（用于SPEKE）
// 返回32字节私钥和32字节公钥
// 注意：与HarmonyOS pake_protocol_ec_common.c:37-46保持一致，进行私钥clamping
func generateX25519KeyPair() ([]byte, []byte, error) {
	// 生成32字节随机私钥
	privateKey := make([]byte, 32)
	_, err := rand.Read(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("生成X25519私钥失败: %w", err)
	}

	// X25519私钥clamping（与HarmonyOS一致）
	// 参考：pake_protocol_ec_common.c lines 43-45
	// #define PAKE_PRIVATE_KEY_AND_MASK_HIGH 0xF8
	// #define PAKE_PRIVATE_KEY_AND_MASK_LOW  0x7F
	// #define PAKE_PRIVATE_KEY_OR_MASK_LOW   0x40
	privateKey[31] &= 0xF8 // 清除最高字节的低3位
	privateKey[0] &= 0x7F  // 清除最低字节的最高位
	privateKey[0] |= 0x40  // 设置最低字节的第6位

	// 计算公钥：publicKey = privateKey * G
	publicKey, err := curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		return nil, nil, fmt.Errorf("计算X25519公钥失败: %w", err)
	}

	return privateKey, publicKey, nil
}

// ComputeX25519PublicKey 计算X25519的SPEKE公钥
// epk = esk * base（base是从PSK派生的基点）
func computeX25519PublicKey(privateKey []byte, basePoint []byte) ([]byte, error) {
	if len(privateKey) != 32 {
		return nil, fmt.Errorf("私钥长度错误: 期望32字节，实际%d", len(privateKey))
	}
	if len(basePoint) != 32 {
		return nil, fmt.Errorf("base点长度错误: 期望32字节，实际%d", len(basePoint))
	}

	// epk = esk * base
	publicKey, err := curve25519.X25519(privateKey, basePoint)
	if err != nil {
		return nil, fmt.Errorf("计算X25519公钥失败: %w", err)
	}

	return publicKey, nil
}

// computeX25519SharedSecret 计算共享密钥
// sharedSecret = eskSelf * epkPeer
func computeX25519SharedSecret(privateKey, peerPublicKey []byte) ([]byte, error) {
	if len(privateKey) != 32 {
		return nil, fmt.Errorf("私钥长度错误: 期望32字节，实际%d", len(privateKey))
	}
	if len(peerPublicKey) != 32 {
		return nil, fmt.Errorf("对端公钥长度错误: 期望32字节，实际%d", len(peerPublicKey))
	}

	sharedSecret, err := curve25519.X25519(privateKey, peerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("计算X25519共享密钥失败: %w", err)
	}

	return sharedSecret, nil
}

// deriveSessionKey 派生会话密钥（使用 HKDF）
func deriveSessionKey(sharedSecret, salt []byte, keyLen int) ([]byte, error) {
	hkdfReader := hkdf.New(sha256.New, sharedSecret, salt, []byte(HiChainSpekeSessionKeyInfo))

	sessionKey := make([]byte, keyLen)
	_, err := hkdfReader.Read(sessionKey)
	if err != nil {
		return nil, fmt.Errorf("HKDF派生密钥失败: %w", err)
	}

	return sessionKey, nil
}

// elligatorCurve25519 实现 Elligator 2 算法，将 32 字节哈希映射到 Curve25519 点
func elligatorCurve25519(r []byte) ([]byte, error) {
	if len(r) != 32 {
		return nil, fmt.Errorf("输入长度必须为 32 字节，实际: %d", len(r))
	}

	// Curve25519 参数: p = 2^255 - 19
	p := new(big.Int)
	p.SetString("7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffed", 16)

	// (p-1)/2 用于 Legendre 符号
	pMinus1Div2 := new(big.Int)
	pMinus1Div2.SetString("3ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff6", 16)

	// A = 486662
	A := new(big.Int)
	A.SetString("076d06", 16)

	// 清除高位
	rCopy := make([]byte, 32)
	copy(rCopy, r)
	rCopy[31] &= 0x7F

	// 将 r 解释为 field element (little-endian)
	rField := bytesToFieldElement(rCopy, p)

	// 计算 r^2
	r2 := new(big.Int).Mul(rField, rField)
	r2.Mod(r2, p)

	// 计算 1 + 2*r^2
	denom := new(big.Int).Add(big.NewInt(1), new(big.Int).Lsh(r2, 1))
	denom.Mod(denom, p)

	// 计算 -A / (1 + 2*r^2)
	denomInv := new(big.Int).ModInverse(denom, p)
	if denomInv == nil {
		return nil, fmt.Errorf("division by zero in Elligator")
	}

	u := new(big.Int).Mul(new(big.Int).Neg(A), denomInv)
	u.Mod(u, p)

	// 计算 v^2 = u^3 + A*u^2 + u
	u2 := new(big.Int).Mul(u, u)
	u2.Mod(u2, p)
	u3 := new(big.Int).Mul(u2, u)
	u3.Mod(u3, p)
	au2 := new(big.Int).Mul(A, u2)
	au2.Mod(au2, p)
	v2 := new(big.Int).Add(u3, au2)
	v2.Add(v2, u)
	v2.Mod(v2, p)

	// 检查 v^2 是否是二次剩余
	chi := new(big.Int).Exp(v2, pMinus1Div2, p)

	// 如果不是二次剩余，使用 u' = -A - u
	if chi.Cmp(big.NewInt(1)) != 0 {
		uOrig := new(big.Int).Mul(new(big.Int).Neg(A), denomInv)
		uOrig.Mod(uOrig, p)
		u = new(big.Int).Neg(A)
		u.Sub(u, uOrig)
		u.Mod(u, p)
	}

	return fieldElementToBytes(u), nil
}

func reverseBytes(b []byte) []byte {
	result := make([]byte, len(b))
	for i := 0; i < len(b); i++ {
		result[i] = b[len(b)-1-i]
	}
	return result
}

func bytesToFieldElement(b []byte, p *big.Int) *big.Int {
	result := new(big.Int).SetBytes(reverseBytes(b))
	result.Mod(result, p)
	return result
}

func fieldElementToBytes(x *big.Int) []byte {
	b := x.Bytes()
	if len(b) < 32 {
		padded := make([]byte, 32)
		copy(padded[32-len(b):], b)
		b = padded
	}
	return reverseBytes(b)
}

// hexToBytes 十六进制字符串转字节数组
func hexToBytes(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

// bytesToHex 字节数组转十六进制字符串
func bytesToHex(b []byte) string {
	return hex.EncodeToString(b)
}

// ComputeKcfDataV1 计算密钥确认数据（PAKE V1 协议）
// 使用HMAC-SHA256方式
// 根据pake_v1_protocol_common.c实现：
//   - GenerateProof（生成自己的）: message = challengeSelf + challengePeer
//   - VerifyProof（验证对端的）: message = challengePeer + challengeSelf
// 参数：
//   - hmacKey: HMAC密钥
//   - challengeSelf: 自己的challenge
//   - challengePeer: 对端的challenge
//   - isGenerateSelf: true=生成自己的kcfData, false=验证对端的kcfData
func computeKcfDataV1(hmacKey []byte, challengeSelf []byte, challengePeer []byte, isGenerateSelf bool) []byte {
	// 拼接消息
	message := make([]byte, 0, len(challengeSelf)+len(challengePeer))

	if isGenerateSelf {
		// 生成自己的kcfData: message = challengeSelf + challengePeer
		message = append(message, challengeSelf...)
		message = append(message, challengePeer...)
	} else {
		// 验证对端的kcfData: message = challengePeer + challengeSelf
		message = append(message, challengePeer...)
		message = append(message, challengeSelf...)
	}

	// 计算HMAC-SHA256
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write(message)
	return mac.Sum(nil)
}

// EncryptAesGcm 使用 AES-GCM 加密数据
// 参数：
//   - key: 加密密钥（PAKE SessionKey，16或32字节）
//   - nonce: 随机nonce（12字节，GCM标准）
//   - plaintext: 明文数据
//   - aad: Additional Authenticated Data (可选，用于额外认证)
// 返回：加密后的密文（包含GCM tag，但不包含nonce）
func encryptAesGcm(key []byte, nonce []byte, plaintext []byte, aad []byte) ([]byte, error) {
	// 验证密钥长度
	if len(key) != 16 && len(key) != 32 {
		return nil, fmt.Errorf("AES-GCM密钥长度错误: 期望16或32字节，实际%d", len(key))
	}

	// 验证nonce长度（GCM标准使用12字节）
	if len(nonce) < 12 {
		return nil, fmt.Errorf("AES-GCM nonce长度错误: 期望至少12字节，实际%d", len(nonce))
	}

	// 创建AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建AES cipher失败: %w", err)
	}

	// 使用前12字节作为GCM nonce
	gcmNonce := nonce[:12]

	// 创建GCM模式
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建GCM失败: %w", err)
	}

	// 加密（Seal会自动添加authentication tag）
	// 参数：dst, nonce, plaintext, additionalData
	ciphertext := aesgcm.Seal(nil, gcmNonce, plaintext, aad)

	return ciphertext, nil
}

// DecryptAesGcm 使用 AES-GCM 解密数据
// 参数：
//   - key: 解密密钥（PAKE SessionKey，16或32字节）
//   - nonce: 随机nonce（12字节，GCM标准）
//   - ciphertext: 密文数据（包含GCM tag）
//   - aad: Additional Authenticated Data (必须与加密时使用的相同)
// 返回：解密后的明文
func decryptAesGcm(key []byte, nonce []byte, ciphertext []byte, aad []byte) ([]byte, error) {
	// 验证密钥长度
	if len(key) != 16 && len(key) != 32 {
		return nil, fmt.Errorf("AES-GCM密钥长度错误: 期望16或32字节，实际%d", len(key))
	}

	// 验证nonce长度（GCM标准使用12字节）
	if len(nonce) < 12 {
		return nil, fmt.Errorf("AES-GCM nonce长度错误: 期望至少12字节，实际%d", len(nonce))
	}

	// 创建AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建AES cipher失败: %w", err)
	}

	// 使用前12字节作为GCM nonce
	gcmNonce := nonce[:12]

	// 创建GCM模式
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建GCM失败: %w", err)
	}

	// 解密（Open会自动验证authentication tag）
	// 参数：dst, nonce, ciphertext, additionalData
	plaintext, err := aesgcm.Open(nil, gcmNonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("AES-GCM解密失败（可能是密钥错误或数据被篡改）: %w", err)
	}

	return plaintext, nil
}

// ============================================================
// ED25519 密钥生成、签名和验证（用于标准绑定交换）
// ============================================================

// generateED25519KeyPair 生成ED25519密钥对
// 返回: (privateKey, publicKey, error)
// privateKey: 64字节（包含32字节种子+32字节公钥）
// publicKey: 32字节
func generateED25519KeyPair() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("生成ED25519密钥对失败: %w", err)
	}
	return privateKey, publicKey, nil
}

// signED25519 使用ED25519私钥签名（兼容HarmonyOS HUKS）
//   重要：HarmonyOS HUKS的ED25519实现使用SHA256预哈希（HashEdDSA）
// 签名流程：signature = ED25519_Sign(privateKey, SHA256(message))
// 参数:
//   - privateKey: ED25519私钥（64字节）
//   - message: 待签名的消息
// 返回: 签名（64字节）
func signED25519(privateKey ed25519.PrivateKey, message []byte) ([]byte, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("ED25519私钥长度错误: 期望%d字节，实际%d字节", ed25519.PrivateKeySize, len(privateKey))
	}

	//   HarmonyOS HUKS兼容性：先对消息进行SHA256哈希
	// 参考：huks_adapter.c:970 - res = Sha256(message, &messageHash);
	messageHash := sha256.Sum256(message)

	// 对哈希值进行ED25519签名
	signature := ed25519.Sign(privateKey, messageHash[:])

	if len(signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("ED25519签名长度错误: 期望%d字节，实际%d字节", ed25519.SignatureSize, len(signature))
	}

	return signature, nil
}

// verifyED25519Signature 验证ED25519签名（兼容HarmonyOS HUKS）
//   重要：HarmonyOS HUKS的ED25519实现使用SHA256预哈希（HashEdDSA）
// 验证流程：ED25519_Verify(publicKey, SHA256(message), signature)
// 参数:
//   - publicKey: ED25519公钥（32字节）
//   - message: 原始消息
//   - signature: 签名（64字节）
// 返回: 验证是否通过
func verifyED25519Signature(publicKey ed25519.PublicKey, message []byte, signature []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize {
		return false
	}

	if len(signature) != ed25519.SignatureSize {
		return false
	}

	//   HarmonyOS HUKS兼容性：先对消息进行SHA256哈希
	// 参考：huks_adapter.c:970 + huks_adapter.c:1012-1013 (HKS_TAG_DIGEST = HKS_DIGEST_SHA256)
	messageHash := sha256.Sum256(message)

	// 使用哈希值验证签名
	return ed25519.Verify(publicKey, messageHash[:], signature)
}

// GenerateRandomBytes 生成随机字节
func generateRandomBytes(length int) ([]byte, error) {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		return nil, fmt.Errorf("生成随机字节失败: %w", err)
	}
	return bytes, nil
}
