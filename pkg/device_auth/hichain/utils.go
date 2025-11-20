package hichain

import "bytes"

// CleanJSONData 清理JSON数据中的控制字符
// C代码可能在JSON字符串末尾添加\x00等控制字符，需要清理
func CleanJSONData(data []byte) []byte {
	// 查找第一个 \x00（null terminator）
	if idx := bytes.IndexByte(data, 0); idx != -1 {
		return data[:idx]
	}
	return data
}
