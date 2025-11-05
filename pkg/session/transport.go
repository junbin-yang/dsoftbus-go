package session

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/junbin-yang/dsoftbus-go/pkg/utils/crypto"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// processSession 处理会话（握手和数据传输）
func (m *TcpSessionManager) processSession(session *TcpSession) error {
	sessionID := session.GetSessionID()

	// 第一步：处理握手消息
	if err := m.handleRequestMessage(session); err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	logger.Infof("Session %d handshake completed, name=%s", sessionID, session.GetSessionName())

	// 第二步：循环接收数据
	for {
		// 检查会话是否已关闭
		if session.IsClosed() {
			logger.Infof("Session %d is closed, exiting receive loop", sessionID)
			break
		}

		// 接收并处理数据
		data, err := m.receiveSessionData(session)
		if err != nil {
			if err == io.EOF {
				logger.Infof("Session %d connection closed by peer", sessionID)
				break
			}
			// 检查是否是由于正常关闭导致的错误
			if session.IsClosed() {
				logger.Infof("Session %d connection closed normally", sessionID)
				break
			}
			return fmt.Errorf("receive data failed: %w", err)
		}

		// 查找对应的SessionServer并调用回调
		sessionName := session.GetSessionName()
		server := m.GetSessionServer(sessionName)
		if server != nil && server.Listener != nil {
			server.Listener.OnBytesReceived(sessionID, data)
		} else {
			logger.Warnf("No listener for session %d (name=%s)", sessionID, sessionName)
		}
	}

	return nil
}

// handleRequestMessage 处理首次连接的握手消息
func (m *TcpSessionManager) handleRequestMessage(session *TcpSession) error {
	sessionID := session.GetSessionID()

	// 1. 接收AUTH_PACKET头部（24字节）
	headerBuf := make([]byte, AuthPacketHeadSize)
	if _, err := io.ReadFull(session.Conn, headerBuf); err != nil {
		return fmt.Errorf("read auth packet header failed: %w", err)
	}

	// 2. 解析头部
	var authPkt AuthPacket
	buf := bytes.NewReader(headerBuf)
	binary.Read(buf, binary.LittleEndian, &authPkt.Module)
	binary.Read(buf, binary.LittleEndian, &authPkt.Seq)
	binary.Read(buf, binary.LittleEndian, &authPkt.Flags)
	binary.Read(buf, binary.LittleEndian, &authPkt.DataLen)
	binary.Read(buf, binary.LittleEndian, &authPkt.Reserved)

	logger.Debugf("Auth packet: module=%d, seq=%d, dataLen=%d", authPkt.Module, authPkt.Seq, authPkt.DataLen)

	// 3. 接收加密数据
	if authPkt.DataLen == 0 || authPkt.DataLen > RecvBuffSize {
		return fmt.Errorf("invalid data length: %d", authPkt.DataLen)
	}

	encryptedData := make([]byte, authPkt.DataLen)
	if _, err := io.ReadFull(session.Conn, encryptedData); err != nil {
		return fmt.Errorf("read encrypted data failed: %w", err)
	}

	// 4. 解密数据（需要从AuthManager获取会话密钥）
	// 前4字节是会话密钥索引
	if len(encryptedData) < 4 {
		return fmt.Errorf("encrypted data too short")
	}

	keyIndex := binary.LittleEndian.Uint32(encryptedData[:4])
	logger.Debugf("Session key index: %d", keyIndex)

	// 从AuthManager获取会话密钥
	// GetSessionKeyByIndex会返回最新添加的该索引的密钥
	sessionKey, err := m.authMgr.GetSessionKeyByIndex(int(keyIndex))
	if err != nil {
		return fmt.Errorf("get session key failed: %w", err)
	}

	// 解密数据（跳过前4字节的索引）
	plaintext, err := crypto.DecryptAESGCM(sessionKey, encryptedData[4:])
	if err != nil {
		return fmt.Errorf("decrypt failed: %w", err)
	}

	// 5. 解析JSON数据
	var firstPkt FirstPacketData
	if err := json.Unmarshal(plaintext, &firstPkt); err != nil {
		return fmt.Errorf("parse JSON failed: %w", err)
	}

	logger.Debugf("First packet: busName=%s, deviceID=%s", firstPkt.BusName, firstPkt.DeviceID)

	// 6. 更新会话信息
	session.SetSessionName(firstPkt.BusName)
	session.SetDeviceID(firstPkt.DeviceID)
	session.mu.Lock()
	session.BusVersion = firstPkt.BusVersion
	session.mu.Unlock()

	// 7. 解码并设置会话密钥（Base64编码）
	decodedKey, err := base64.StdEncoding.DecodeString(firstPkt.SessionKey)
	if err != nil {
		return fmt.Errorf("decode session key failed: %w", err)
	}
	if len(decodedKey) != SessionKeyLength {
		return fmt.Errorf("invalid session key length: %d", len(decodedKey))
	}

	var sessKey [SessionKeyLength]byte
	copy(sessKey[:], decodedKey)
	session.SetSessionKey(sessKey)

	// 8. 查找对应的SessionServer
	server := m.GetSessionServer(firstPkt.BusName)
	if server == nil {
		return fmt.Errorf("no session server found for %s", firstPkt.BusName)
	}

	// 9. 将会话添加到服务器
	if err := server.AddSession(session); err != nil {
		return fmt.Errorf("add session to server failed: %w", err)
	}

	// 10. 发送响应给客户端
	if err := m.sendResponseToClient(session, 0); err != nil {
		return fmt.Errorf("send response failed: %w", err)
	}

	// 11. 更新会话状态为已打开
	session.SetState(SessionStateOpened)

	// 12. 调用OnSessionOpened回调
	if server.Listener != nil {
		result := server.Listener.OnSessionOpened(sessionID)
		if result != 0 {
			return fmt.Errorf("listener rejected session: %d", result)
		}
	}

	return nil
}

// sendResponseToClient 发送握手响应给客户端
func (m *TcpSessionManager) sendResponseToClient(session *TcpSession, result int) error {
	// 构建响应JSON
	deviceID := "UNKNOWN"
	if m.authMgr != nil {
		deviceID = m.authMgr.GetLocalDeviceID()
	}

	response := ResponsePacketData{
		DeviceID:      deviceID,
		SessionName:   session.GetSessionName(),
		BusVersion:    session.BusVersion,
		MySessionName: session.GetSessionName(),
		Result:        result,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal response failed: %w", err)
	}

	// 获取会话密钥用于加密
	// 注意：响应使用的是session的会话密钥
	sessionKey := session.GetSessionKey()

	// 加密数据
	encryptedData, err := crypto.EncryptAESGCM(sessionKey[:], jsonData)
	if err != nil {
		return fmt.Errorf("encrypt response failed: %w", err)
	}

	// 构建AUTH_PACKET头部
	authPkt := AuthPacket{
		Module:   0,
		Seq:      0,
		Flags:    0,
		DataLen:  uint32(len(encryptedData)),
		Reserved: 0,
	}

	// 发送头部
	headerBuf := new(bytes.Buffer)
	binary.Write(headerBuf, binary.LittleEndian, authPkt.Module)
	binary.Write(headerBuf, binary.LittleEndian, authPkt.Seq)
	binary.Write(headerBuf, binary.LittleEndian, authPkt.Flags)
	binary.Write(headerBuf, binary.LittleEndian, authPkt.DataLen)
	binary.Write(headerBuf, binary.LittleEndian, authPkt.Reserved)

	if _, err := session.Conn.Write(headerBuf.Bytes()); err != nil {
		return fmt.Errorf("write header failed: %w", err)
	}

	// 发送加密数据
	if _, err := session.Conn.Write(encryptedData); err != nil {
		return fmt.Errorf("write encrypted data failed: %w", err)
	}

	logger.Debugf("Response sent to session %d", session.GetSessionID())
	return nil
}

// receiveSessionData 接收并解密会话数据
func (m *TcpSessionManager) receiveSessionData(session *TcpSession) ([]byte, error) {
	// 1. 接收传输包头部（16字节）
	headerBuf := make([]byte, TransPacketHeadSize)
	n, err := io.ReadFull(session.Conn, headerBuf)
	if err != nil {
		logger.Debugf("Session %d read header failed: read %d bytes, err=%v", session.GetSessionID(), n, err)
		return nil, err
	}

	// 2. 解析头部
	var transPkt TransPacket
	buf := bytes.NewReader(headerBuf)
	binary.Read(buf, binary.LittleEndian, &transPkt.MagicNum)
	binary.Read(buf, binary.LittleEndian, &transPkt.SeqNum)
	binary.Read(buf, binary.LittleEndian, &transPkt.Flags)
	binary.Read(buf, binary.LittleEndian, &transPkt.DataLen)

	logger.Debugf("Session %d received header: magic=0x%X, seq=%d, flags=%d, dataLen=%d",
		session.GetSessionID(), transPkt.MagicNum, transPkt.SeqNum, transPkt.Flags, transPkt.DataLen)

	// 3. 验证魔数
	if transPkt.MagicNum != PkgHeaderIdentifier {
		return nil, fmt.Errorf("invalid magic number: 0x%X", transPkt.MagicNum)
	}

	// 4. 验证数据长度
	if transPkt.DataLen == 0 || transPkt.DataLen > RecvBuffSize {
		return nil, fmt.Errorf("invalid data length: %d", transPkt.DataLen)
	}

	// 5. 接收加密数据（IV + 密文 + MAC）
	encryptedData := make([]byte, transPkt.DataLen)
	if _, err := io.ReadFull(session.Conn, encryptedData); err != nil {
		return nil, fmt.Errorf("read encrypted data failed: %w", err)
	}

	// 6. 解密数据
	sessionKey := session.GetSessionKey()
	plaintext, err := crypto.DecryptAESGCM(sessionKey[:], encryptedData)
	if err != nil {
		return nil, fmt.Errorf("decrypt data failed: %w", err)
	}

	// 7. 防重放检查
	if !session.CheckAndAddRecvSeqNum(transPkt.SeqNum) {
		return nil, ErrReplayAttack
	}

	// 8. 更新最后活跃时间
	session.UpdateLastActive()

	logger.Debugf("Received %d bytes from session %d (seq=%d)", len(plaintext), session.GetSessionID(), transPkt.SeqNum)

	return plaintext, nil
}

// SendBytes 发送数据到会话
func (m *TcpSessionManager) SendBytes(sessionID int, data []byte) error {
	// 1. 获取会话
	session, err := m.GetSession(sessionID)
	if err != nil {
		return err
	}

	// 2. 检查会话状态
	if !session.IsOpened() {
		return ErrSessionClosed
	}

	// 3. 检查数据长度
	if len(data) == 0 {
		return ErrInvalidParameter
	}
	if len(data) > SendBuffMaxSize {
		return ErrDataTooLarge
	}

	// 4. 获取会话密钥并加密数据
	sessionKey := session.GetSessionKey()
	encryptedData, err := crypto.EncryptAESGCM(sessionKey[:], data)
	if err != nil {
		return fmt.Errorf("encrypt data failed: %w", err)
	}

	// 5. 获取下一个序列号
	seqNum := session.NextSendSeqNum()

	// 6. 构建传输包头部
	transPkt := TransPacket{
		MagicNum: PkgHeaderIdentifier,
		SeqNum:   seqNum,
		Flags:    0,
		DataLen:  uint32(len(encryptedData)),
	}

	logger.Debugf("Session %d sending: magic=0x%X, seq=%d, flags=%d, dataLen=%d (plaintext=%d bytes)",
		sessionID, transPkt.MagicNum, transPkt.SeqNum, transPkt.Flags, transPkt.DataLen, len(data))

	// 7. 序列化头部
	headerBuf := new(bytes.Buffer)
	binary.Write(headerBuf, binary.LittleEndian, transPkt.MagicNum)
	binary.Write(headerBuf, binary.LittleEndian, transPkt.SeqNum)
	binary.Write(headerBuf, binary.LittleEndian, transPkt.Flags)
	binary.Write(headerBuf, binary.LittleEndian, transPkt.DataLen)

	// 8. 发送头部
	n, err := session.Conn.Write(headerBuf.Bytes())
	if err != nil {
		return fmt.Errorf("write header failed: %w", err)
	}
	logger.Debugf("Session %d wrote %d header bytes", sessionID, n)

	// 9. 发送加密数据
	n, err = session.Conn.Write(encryptedData)
	if err != nil {
		return fmt.Errorf("write encrypted data failed: %w", err)
	}
	logger.Debugf("Session %d wrote %d encrypted data bytes", sessionID, n)

	// 10. 更新最后活跃时间
	session.UpdateLastActive()

	logger.Infof("Session %d sent %d bytes successfully (seq=%d)", sessionID, len(data), seqNum)

	return nil
}

// CloseSession 关闭会话
func (m *TcpSessionManager) CloseSession(sessionID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessionMap[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	// 从SessionServer中移除
	sessionName := session.GetSessionName()
	if sessionName != DefaultUnknownSessionName {
		for _, server := range m.serverMap {
			if server.SessionName == sessionName {
				server.RemoveSession(sessionID)
				// 调用OnSessionClosed回调
				if server.Listener != nil {
					server.Listener.OnSessionClosed(sessionID)
				}
				break
			}
		}
	}

	// 关闭会话
	session.Close()

	// 从映射中删除
	delete(m.sessionMap, sessionID)

	logger.Infof("Session %d closed", sessionID)

	return nil
}

// sendRequestMessage 客户端发送会话请求（握手）
func (m *TcpSessionManager) sendRequestMessage(session *TcpSession) error {
	// 1. 生成会话密钥（32字节随机密钥）
	sessionKey, err := crypto.GenerateRandomBytes(SessionKeyLength)
	if err != nil {
		return fmt.Errorf("generate session key failed: %w", err)
	}

	var sessKey [SessionKeyLength]byte
	copy(sessKey[:], sessionKey)
	session.SetSessionKey(sessKey)

	// 2. 构建FirstPacketData
	deviceID := "UNKNOWN"
	if m.authMgr != nil {
		deviceID = m.authMgr.GetLocalDeviceID()
	}

	firstPkt := FirstPacketData{
		BusName:    session.GetSessionName(),
		DeviceID:   deviceID,
		SessionKey: base64.StdEncoding.EncodeToString(sessionKey),
		BusVersion: DefaultBusVersion,
	}

	jsonData, err := json.Marshal(firstPkt)
	if err != nil {
		return fmt.Errorf("marshal first packet failed: %w", err)
	}

	// 3. 从AuthManager获取认证会话密钥进行加密
	// 使用索引0的会话密钥（在认证阶段协商的密钥）
	authSessionKey, err := m.authMgr.GetSessionKeyByIndex(0)
	if err != nil {
		return fmt.Errorf("get auth session key failed: %w", err)
	}

	// 4. 加密数据
	encryptedData, err := crypto.EncryptAESGCM(authSessionKey, jsonData)
	if err != nil {
		return fmt.Errorf("encrypt first packet failed: %w", err)
	}

	// 5. 添加密钥索引前缀（4字节）
	dataWithIndex := make([]byte, 4+len(encryptedData))
	binary.LittleEndian.PutUint32(dataWithIndex[:4], 0) // 使用索引0
	copy(dataWithIndex[4:], encryptedData)

	// 6. 构建AUTH_PACKET头部
	authPkt := AuthPacket{
		Module:   0,
		Seq:      0,
		Flags:    0,
		DataLen:  uint32(len(dataWithIndex)),
		Reserved: 0,
	}

	// 7. 序列化并发送头部
	headerBuf := new(bytes.Buffer)
	binary.Write(headerBuf, binary.LittleEndian, authPkt.Module)
	binary.Write(headerBuf, binary.LittleEndian, authPkt.Seq)
	binary.Write(headerBuf, binary.LittleEndian, authPkt.Flags)
	binary.Write(headerBuf, binary.LittleEndian, authPkt.DataLen)
	binary.Write(headerBuf, binary.LittleEndian, authPkt.Reserved)

	if _, err := session.Conn.Write(headerBuf.Bytes()); err != nil {
		return fmt.Errorf("write header failed: %w", err)
	}

	// 8. 发送加密数据
	if _, err := session.Conn.Write(dataWithIndex); err != nil {
		return fmt.Errorf("write encrypted data failed: %w", err)
	}

	logger.Infof("Client session %d sent handshake request", session.GetSessionID())

	// 9. 接收服务端响应
	if err := m.receiveHandshakeResponse(session); err != nil {
		return fmt.Errorf("receive handshake response failed: %w", err)
	}

	// 10. 更新会话状态为已打开
	session.SetState(SessionStateOpened)

	logger.Infof("Client session %d handshake completed", session.GetSessionID())

	return nil
}

// receiveHandshakeResponse 接收服务端的握手响应
func (m *TcpSessionManager) receiveHandshakeResponse(session *TcpSession) error {
	// 1. 接收AUTH_PACKET头部（24字节）
	headerBuf := make([]byte, AuthPacketHeadSize)
	if _, err := io.ReadFull(session.Conn, headerBuf); err != nil {
		return fmt.Errorf("read response header failed: %w", err)
	}

	// 2. 解析头部
	var authPkt AuthPacket
	buf := bytes.NewReader(headerBuf)
	binary.Read(buf, binary.LittleEndian, &authPkt.Module)
	binary.Read(buf, binary.LittleEndian, &authPkt.Seq)
	binary.Read(buf, binary.LittleEndian, &authPkt.Flags)
	binary.Read(buf, binary.LittleEndian, &authPkt.DataLen)
	binary.Read(buf, binary.LittleEndian, &authPkt.Reserved)

	// 3. 接收加密数据
	if authPkt.DataLen == 0 || authPkt.DataLen > RecvBuffSize {
		return fmt.Errorf("invalid response data length: %d", authPkt.DataLen)
	}

	encryptedData := make([]byte, authPkt.DataLen)
	if _, err := io.ReadFull(session.Conn, encryptedData); err != nil {
		return fmt.Errorf("read encrypted response failed: %w", err)
	}

	// 4. 使用会话密钥解密（客户端生成的会话密钥）
	sessionKey := session.GetSessionKey()
	plaintext, err := crypto.DecryptAESGCM(sessionKey[:], encryptedData)
	if err != nil {
		return fmt.Errorf("decrypt response failed: %w", err)
	}

	// 5. 解析响应JSON
	var response ResponsePacketData
	if err := json.Unmarshal(plaintext, &response); err != nil {
		return fmt.Errorf("parse response JSON failed: %w", err)
	}

	// 6. 检查结果
	if response.Result != 0 {
		return fmt.Errorf("server rejected session: result=%d", response.Result)
	}

	logger.Infof("Client session %d received handshake response from device %s",
		session.GetSessionID(), response.DeviceID)

	return nil
}

// receiveLoop 客户端数据接收循环
func (m *TcpSessionManager) receiveLoop(session *TcpSession) error {
	sessionID := session.GetSessionID()

	for {
		// 检查会话是否已关闭
		if session.IsClosed() {
			logger.Infof("Client session %d is closed, exiting receive loop", sessionID)
			break
		}

		// 接收并处理数据
		data, err := m.receiveSessionData(session)
		if err != nil {
			if err == io.EOF {
				logger.Infof("Client session %d connection closed by peer", sessionID)
				break
			}
			// 检查是否是由于正常关闭导致的错误
			if session.IsClosed() {
				logger.Infof("Client session %d connection closed normally", sessionID)
				break
			}
			return fmt.Errorf("receive data failed: %w", err)
		}

		// 查找对应的SessionServer并调用回调
		// 注意：客户端可能没有对应的SessionServer，这是正常的
		sessionName := session.GetSessionName()
		server := m.GetSessionServer(sessionName)
		if server != nil && server.Listener != nil {
			server.Listener.OnBytesReceived(sessionID, data)
		} else {
			// 如果没有监听器，只记录日志
			logger.Infof("Client session %d received %d bytes (no listener)", sessionID, len(data))
		}
	}

	return nil
}
