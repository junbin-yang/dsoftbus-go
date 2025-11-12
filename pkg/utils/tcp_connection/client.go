package tcp_connection

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

type BaseClient struct {
	conn     net.Conn           // 与远程设备的网络连接
	handler  *BaseClientHandler // 客户端事件回调处理器
	stopChan chan struct{}      // 停止信号通道
	wg       sync.WaitGroup     // 等待goroutine退出
	mu       sync.Mutex         // 保护conn和handler的锁
}

// GetBaseClient 获取客户端实例
func GetBaseClient(handler *BaseClientHandler) *BaseClient {
	return &BaseClient{
		handler:  handler,
		stopChan: make(chan struct{}),
	}
}

// Connect 连接到远程设备
// 参数：
//   - remoteIP：远程设备IP地址
//   - remotePort：远程设备端口
// 返回：
//   - 错误信息（连接失败时）
func (c *BaseClient) Connect(remoteIP string, remotePort int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return fmt.Errorf("已建立连接")
	}
	if c.handler == nil {
		return fmt.Errorf("未设置回调处理器")
	}

	addr := fmt.Sprintf("%s:%d", remoteIP, remotePort)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("连接到%s失败: %v", addr, err)
	}

	c.conn = conn
	// 启动数据接收循环
	c.wg.Add(1)
	go c.handleDataEvents()
	// 触发连接成功回调
	c.handler.onConnect()
	return nil
}

// Close 关闭与远程设备的连接
func (c *BaseClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return
	}

	// 发送停止信号
	close(c.stopChan)
	// 关闭连接
	c.conn.Close()
	c.conn = nil
	// 等待接收循环退出
	c.wg.Wait()
	// 触发断开连接回调
	if c.handler != nil && c.handler.onDisconnect != nil {
		c.handler.onDisconnect()
	}
}

// SendBytes 发送数据
// 参数：
//   - data：要发送的数据
// 返回：
//   - 错误信息（发送失败时）
func (c *BaseClient) SendBytes(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("未建立连接")
	}
	_, err := c.conn.Write(data)
	if err != nil {
		return fmt.Errorf("发送数据失败: %v", err)
	}
	return nil
}

// handleDataEvents 循环接收数据并触发回调
func (c *BaseClient) handleDataEvents() {
	defer c.wg.Done()

	buf := make([]byte, DefaultBufSize)
	used := 0 // 缓冲区已使用字节数

	for {
		select {
		case <-c.stopChan: // 收到停止信号，退出循环
			return
		default:
		}

		// 读取数据（阻塞模式，与服务端保持一致）
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			return
		}

		n, err := conn.Read(buf[used:])
		if err != nil {
			// 处理读取错误
			if err == io.EOF {
				log.Debug("[BASE_CLIENT] 连接正常关闭")
			} else {
				log.Errorf("[BASE_CLIENT] 读取数据错误: %v", err)
			}
			return
		}

		if n == 0 {
			c.Close()
			return
		}

		used += n

		// 调用数据处理回调
		processed := c.handler.processPackets(buf, used)
		if processed > 0 {
			// 移动未处理数据到缓冲区头部
			used -= processed
			if used > 0 {
				copy(buf, buf[processed:processed+used])
			}
		} else if processed < 0 {
			// 处理失败，关闭连接
			log.Error("[BASE_CLIENT] 数据包处理失败，关闭连接")
			c.Close()
			return
		}

		// 检查缓冲区溢出
		if used >= len(buf) {
			log.Errorf("[BASE_CLIENT] 缓冲区溢出，关闭连接")
			c.Close()
			return
		}
	}
}
