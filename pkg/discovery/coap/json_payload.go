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
	CapabilityBitmap []uint32
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
		jsonDeviceID:     dev.DeviceId,
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

// cleanJSONPayload 清理无效控制字符（含\u0000）
func cleanJSONPayload(payload []byte) []byte {
	var cleaned bytes.Buffer
	for i := 0; i < len(payload); i++ {
		b := payload[i]
		// 保留有效ASCII可见字符及JSON必需控制符
		if (b >= 0x20 && b <= 0x7E) || b == 0x0A || b == 0x0D || b == 0x09 {
			cleaned.WriteByte(b)
		}
	}
	return cleaned.Bytes()
}

// ParseServiceDiscover 解析对端的设备发现 JSON 字符串，返回 remoteUrl（coapUri）
func ParseServiceDiscover(buf []byte, out *DeviceInfo) (string, error) {
	if len(buf) == 0 || out == nil {
		return "", errors.New("invalid argument")
	}

	buf = cleanJSONPayload(buf)

	var data map[string]any
	if err := json.Unmarshal(buf, &data); err != nil {
		return "", err
	}

	// 基本字段解析
	if v, ok := data[jsonDeviceID].(string); ok && v != "" {
		out.DeviceId = v
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
		caps := make([]uint32, 0, len(arr))
		for _, it := range arr {
			if f, ok := it.(float64); ok && f >= 0 {
				caps = append(caps, uint32(f))
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
