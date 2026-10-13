package desktop

import "time"

// 桌面、顯示模式與裝置切換是暫時狀態，應快速重建 duplication；
// 不支援的後端仍採較長退避，避免一直探測及重建裝置。
func dxgiRetryDelay(hr uint32) time.Duration {
	switch hr {
	case 0x887A0026, // DXGI_ERROR_ACCESS_LOST
		0x887A0005, // DXGI_ERROR_DEVICE_REMOVED
		0x887A0007, // DXGI_ERROR_DEVICE_RESET
		0x887A0022, // DXGI_ERROR_NOT_CURRENTLY_AVAILABLE
		0x887A0028, // DXGI_ERROR_SESSION_DISCONNECTED
		0x80070005: // E_ACCESSDENIED（桌面交接期間）
		return time.Second
	default:
		return 30 * time.Second
	}
}
