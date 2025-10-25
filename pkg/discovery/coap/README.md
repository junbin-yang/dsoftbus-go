# CoAP包

提供基于coap协议的设备发现能力。

## 使用说明

- 通过RegisterProviders注册本地设备信息、本地IP和设备发现回调的提供者

- 通过CoapInitDiscovery初始化，并开始接收广播

- 当有其他设备主动发起发现请求时，会响应并触发回调

- 通过BuildDiscoverPacket编码设备请求包，使用CoapCreateUDPClient和CoapSocketSend发起一次设备查找
