package coap

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
)

const (
	jsonCoapURI          = "coapUri"
	jsonCapabilityBitmap = "capabilityBitmap"
	jsonDeviceID         = "deviceId"
	jsonDeviceName       = "devicename"
	jsonDeviceWlanIP     = "wlanIp"
	jsonDeviceType       = "type"
	jsonHiComVersion     = "hicomversion"
	jsonRequestMode      = "mode"
	jsonDeviceHash       = "deviceHash"
	jsonServiceData      = "serviceData"
)

type NetworkInfo struct {
	IP net.IP
}

type NetChannelInfo struct {
	Network NetworkInfo
}

type DeviceInfo struct {
	DeviceId         string
	DeviceName       string
	DeviceType       uint8
	Version          string
	Mode             uint8
	DeviceHash       string
	ServiceData      string
	CapabilityBitmap []uint16
	NetChannelInfo   NetChannelInfo
}

// PrepareServiceDiscover 生成设备发现 JSON 负载
func PrepareServiceDiscover(isBroadcast bool) (string, error) {
	if localDeviceInfoProvider == nil || localIPStringProvider == nil {
		return "", errors.New("provider not registered")
	}
	dev := localDeviceInfoProvider()
	if dev == nil {
		return "", errors.New("device info is nil")
	}
	ip, err := localIPStringProvider()
	if err != nil || ip == "" {
		return "", errors.New("get local ip failed")
	}

	data := map[string]any{
		jsonDeviceID:     FormatDeviceID(dev.DeviceId), // 格式化为JSON格式以兼容真实鸿蒙
		jsonDeviceName:   dev.DeviceName,
		jsonDeviceType:   dev.DeviceType,
		jsonHiComVersion: dev.Version,
		jsonRequestMode:  dev.Mode,
		jsonDeviceHash:   dev.DeviceHash,
		jsonServiceData:  dev.ServiceData,
		jsonDeviceWlanIP: ip,
	}

	if isBroadcast {
		data[jsonCoapURI] = fmt.Sprintf("coap://%s/%s", ip, COAP_DEVICE_DISCOVER_URI)
	}

	if len(dev.CapabilityBitmap) > 0 {
		data[jsonCapabilityBitmap] = dev.CapabilityBitmap
	}

	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	b = append(b, 0)
	return string(b), nil
}

// CleanJSONData 清理JSON数据中的无效控制字符（包括\x00等）
// 这是导出函数，可供其他包使用
// 参数：
//   - payload：原始JSON字节数组
// 返回：
//   - 清理后的JSON字节数组
func CleanJSONData(payload []byte) []byte {
	var cleaned bytes.Buffer
	for i := 0; i < len(payload); i++ {
		b := payload[i]
		// 保留有效ASCII可见字符及JSON必需控制符
		// 0x20-0x7E: 可见ASCII字符（空格到~）
		// 0x0A: 换行符 \n
		// 0x0D: 回车符 \r
		// 0x09: 制表符 \t
		if (b >= 0x20 && b <= 0x7E) || b == 0x0A || b == 0x0D || b == 0x09 {
			cleaned.WriteByte(b)
		}
	}
	return cleaned.Bytes()
}

// FormatDeviceID 将纯UDID格式化为JSON格式（兼容真实鸿蒙设备）
// 真实鸿蒙设备的deviceId格式是：{"UDID":"xxx"}
// 参数：
//   - udid：纯UDID字符串
// 返回：
//   - JSON格式的设备ID字符串
func FormatDeviceID(udid string) string {
	// 如果已经是JSON格式，直接返回
	if len(udid) > 0 && udid[0] == '{' {
		return udid
	}

	// 格式化为JSON格式
	formatted := fmt.Sprintf(`{"UDID":"%s"}`, udid)
	return formatted
}

// ExtractDeviceID 从可能包含JSON格式的设备ID中提取实际的UDID
// 真实鸿蒙设备的deviceId格式可能是：{"UDID":"xxx"}
// 参数：
//   - deviceID：可能是纯字符串或JSON格式的设备ID
// 返回：
//   - 提取出的实际设备ID
func ExtractDeviceID(deviceID string) string {
	// 尝试解析为JSON格式
	var udidObj struct {
		UDID string `json:"UDID"`
	}

	if err := json.Unmarshal([]byte(deviceID), &udidObj); err == nil && udidObj.UDID != "" {
		// 成功解析，返回UDID
		return udidObj.UDID
	}

	// 无法解析或不是JSON格式，返回原始字符串
	return deviceID
}

// ParseServiceDiscover 解析对端的设备发现 JSON 字符串，返回 remoteUrl（coapUri）
func ParseServiceDiscover(buf []byte, out *DeviceInfo) (string, error) {
	if len(buf) == 0 || out == nil {
		return "", errors.New("invalid argument")
	}

	buf = CleanJSONData(buf)

	var data map[string]any
	if err := json.Unmarshal(buf, &data); err != nil {
		return "", err
	}

	// 基本字段解析
	if v, ok := data[jsonDeviceID].(string); ok && v != "" {
		// 提取真实的设备ID（处理{"UDID":"xxx"}格式）
		out.DeviceId = ExtractDeviceID(v)
	} else {
		return "", errors.New("invalid deviceId")
	}

	if v, ok := data[jsonDeviceName].(string); ok && v != "" {
		out.DeviceName = v
	} else {
		return "", errors.New("invalid devicename")
	}

	if vv, ok := data[jsonDeviceType].(float64); ok && vv >= 0 && vv <= 0xFF {
		out.DeviceType = uint8(vv)
	} else {
		return "", errors.New("invalid device type")
	}

	if v, ok := data[jsonHiComVersion].(string); ok && v != "" {
		out.Version = v
	}

	if vv, ok := data[jsonRequestMode].(float64); ok && vv >= 0 {
		out.Mode = uint8(vv)
	}

	if v, ok := data[jsonDeviceHash].(string); ok && v != "" {
		out.DeviceHash = v
	}

	if v, ok := data[jsonServiceData].(string); ok && v != "" {
		out.ServiceData = v
	}

	if arr, ok := data[jsonCapabilityBitmap].([]any); ok {
		caps := make([]uint16, 0, len(arr))
		for _, it := range arr {
			if f, ok := it.(float64); ok && f >= 0 {
				caps = append(caps, uint16(f))
			}
		}
		out.CapabilityBitmap = caps
	}

	if v, ok := data[jsonDeviceWlanIP].(string); ok && v != "" {
		ip := net.ParseIP(v)
		if ip != nil {
			out.NetChannelInfo.Network.IP = ip
		}
	}

	var remoteUrl string = ""
	if v, ok := data[jsonCoapURI].(string); ok && v != "" {
		remoteUrl = v
	}

	return remoteUrl, nil
}
