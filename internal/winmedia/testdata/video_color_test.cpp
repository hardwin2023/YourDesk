// 執行正式 metadata→D3D 色彩對應及 CPU 像素轉換，不需要 Windows／GPU。
#include <cassert>
#include <cstdint>
#include <cstdlib>
#include <cstdio>
#include <initializer_list>
#include <vector>
using HRESULT=std::int32_t;
using UINT32=std::uint32_t;
constexpr HRESULT S_OK=0,E_INVALIDARG=-1,E_NOTIMPL=-2,MF_E_ATTRIBUTENOTFOUND=-3;
#define FAILED(hr) ((hr)<0)
enum {MFVideoTransferMatrix_Unknown=0,MFVideoTransferMatrix_BT709=1,MFVideoTransferMatrix_BT601=2};
enum {MFNominalRange_Unknown=0,MFNominalRange_0_255=1,MFNominalRange_16_235=2};
enum {D3D11_VIDEO_PROCESSOR_NOMINAL_RANGE_16_235=1,D3D11_VIDEO_PROCESSOR_NOMINAL_RANGE_0_255=2};
enum {MF_MT_YUV_MATRIX=1,MF_MT_VIDEO_NOMINAL_RANGE=2};
struct D3D11_VIDEO_PROCESSOR_COLOR_SPACE {
 unsigned Usage:1,RGB_Range:1,YCbCr_Matrix:1,YCbCr_xvYCC:1,Nominal_Range:2,Reserved:26;
};
struct IMFMediaType {
 UINT32 matrix=MFVideoTransferMatrix_Unknown,range=MFNominalRange_Unknown;
 bool hasMatrix=true,hasRange=true;
 HRESULT error=S_OK;
 HRESULT GetUINT32(int key,UINT32 *out) {
  if(FAILED(error))return error;
  if(key==MF_MT_YUV_MATRIX){if(!hasMatrix)return MF_E_ATTRIBUTENOTFOUND;*out=matrix;return S_OK;}
  assert(key==MF_MT_VIDEO_NOMINAL_RANGE);
  if(!hasRange)return MF_E_ATTRIBUTENOTFOUND;*out=range;return S_OK;
 }
};
#include "../video_color.h"
#include "../encoder_controls.h"
static void near(const unsigned char *p,int r,int g,int b){
 assert(std::abs((int)p[0]-r)<=2 && std::abs((int)p[1]-g)<=2 && std::abs((int)p[2]-b)<=2 && p[3]==255);
}
int main(){
 IMFMediaType type;
 D3D11_VIDEO_PROCESSOR_COLOR_SPACE color{};
 for(unsigned matrix: {1u,2u})for(unsigned range: {1u,2u}){
  type.matrix=matrix;type.range=range;
  assert(yd_media_yuv_color(&type,&color)==S_OK);
  assert(color.YCbCr_Matrix==(matrix==MFVideoTransferMatrix_BT709?1u:0u));
  assert(color.Nominal_Range==(range==MFNominalRange_0_255?2u:1u));
  assert(color.RGB_Range==0 && color.Reserved==0);
  unsigned char pixel[4];
  bool full=range==MFNominalRange_0_255;
  yd_yuv_to_rgba(full?0:16,128,128,color,pixel);near(pixel,0,0,0);
  yd_yuv_to_rgba(full?255:235,128,128,color,pixel);near(pixel,255,255,255);
  // 彩色向量：只測灰階抓不到 601/709 矩陣混用。
  if(matrix==MFVideoTransferMatrix_BT709){
   yd_yuv_to_rgba(full?54:63,full?99:102,full?255:240,color,pixel);near(pixel,255,0,0);
   yd_yuv_to_rgba(full?182:173,full?30:42,full?12:26,color,pixel);near(pixel,0,255,0);
  }else{
   yd_yuv_to_rgba(full?76:81,full?85:90,full?255:240,color,pixel);near(pixel,255,0,0);
  }
 }
 type.hasMatrix=type.hasRange=false;
 assert(yd_media_yuv_color(&type,&color)==S_OK && color.YCbCr_Matrix==1 && color.Nominal_Range==1);
 type.hasMatrix=type.hasRange=true;type.matrix=type.range=0;
 assert(yd_media_yuv_color(&type,&color)==S_OK && color.YCbCr_Matrix==1 && color.Nominal_Range==1);
 type.matrix=4;type.range=2;assert(yd_media_yuv_color(&type,&color)==E_NOTIMPL);
 type.matrix=1;type.range=3;assert(yd_media_yuv_color(&type,&color)==E_NOTIMPL);
 type.error=E_INVALIDARG;assert(yd_media_yuv_color(&type,&color)==E_INVALIDARG);
 assert(yd_media_yuv_color(nullptr,&color)==E_INVALIDARG);
 // RGBA→BGRA 不改 alpha／列間距，也不能將紅藍色反轉到輸出畫面。
 const unsigned char rgba[]={255,0,0,255,0,255,0,127,42,42,42,42,0,0,255,200,255,255,255,255};
 unsigned char bgra[20]{};
 yd_rgba_to_bgra(rgba,12,bgra,12,2,2);
 assert(bgra[0]==0&&bgra[1]==0&&bgra[2]==255&&bgra[3]==255);
 assert(bgra[4]==0&&bgra[5]==255&&bgra[6]==0&&bgra[7]==127);
 assert(bgra[8]==0&&bgra[11]==0&&bgra[12]==255&&bgra[14]==0&&bgra[15]==200);
 assert(bgra[16]==255&&bgra[19]==255);
 assert(yd_encoder_bitrate(5000000,1920,1080,60,70)==5000000);
 assert(yd_encoder_bitrate(0,128,128,30,70)==1000000);
 assert(yd_encoder_bitrate(0,1920,1080,30,70)==5598720);
 assert(yd_encoder_bitrate(0,8192,8192,240,100)>=1000000);
 std::vector<yd_encoder_control> attempts;
 auto choose=[&](yd_encoder_control accepted){
  return [&,accepted](yd_encoder_control mode)->HRESULT{attempts.push_back(mode);return mode==accepted?S_OK:E_INVALIDARG;};
 };
 assert(yd_try_encoder_controls(0,choose(yd_encoder_control::quality))==S_OK&&attempts.size()==1);
 attempts.clear();
 assert(yd_try_encoder_controls(0,choose(yd_encoder_control::cbr))==S_OK&&attempts.size()==2);
 assert(attempts[0]==yd_encoder_control::quality&&attempts[1]==yd_encoder_control::cbr);
 attempts.clear();
 assert(yd_try_encoder_controls(0,choose(yd_encoder_control::media_type))==S_OK&&attempts.size()==3);
 attempts.clear();
 assert(yd_try_encoder_controls(5000000,choose(yd_encoder_control::cbr))==S_OK&&attempts.size()==1);
 assert(attempts[0]==yd_encoder_control::cbr);
 attempts.clear();
 assert(yd_try_encoder_controls(5000000,choose(yd_encoder_control::media_type))==S_OK&&attempts.size()==2);
 assert(yd_try_encoder_controls(0,[](yd_encoder_control)->HRESULT{return E_INVALIDARG;})==E_INVALIDARG);
 std::puts("video color metadata and pixel tests passed");
}
