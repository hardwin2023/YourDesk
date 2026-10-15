#ifndef YOURDESK_ENCODER_CONTROLS_H
#define YOURDESK_ENCODER_CONTROLS_H
#include <algorithm>
#include <cstdint>

enum class yd_encoder_control { quality, cbr, media_type };

// 品質模式的媒體型別也需合理碼率；不能以 1 bps 讓驅動拒絕整個硬體編碼器。
static inline std::uint32_t yd_encoder_bitrate(int requested,int w,int h,int fps,int quality) {
 if(requested>0)return static_cast<std::uint32_t>(requested);
 const auto pixels=static_cast<std::uint64_t>(w)*h;
 const auto nominal=pixels*std::max(1,fps)*(std::clamp(quality,1,100)+20)/1000;
 return static_cast<std::uint32_t>(std::clamp<std::uint64_t>(nominal,1000000,UINT32_MAX));
}

// 同一個硬體 MFT 依序協商品質、CBR、媒體型別預設；不把控制項不支援當成無硬體。
template<class Configure> static HRESULT yd_try_encoder_controls(int requested,Configure configure) {
 if(requested<=0){HRESULT hr=configure(yd_encoder_control::quality);if(!FAILED(hr))return hr;}
 HRESULT hr=configure(yd_encoder_control::cbr);if(!FAILED(hr))return hr;
 return configure(yd_encoder_control::media_type);
}
#endif
