import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

/// 推定に送る画像の長辺の上限。
///
/// **大きい画像は精度を上げない**（API 側で縮小される）。
/// 送信時間とトークンだけが増える。要件 N-06 のコスト試算もこの前提。
let estimateMaxPixels = 1024

/// 推定に送る前に画像を縮める（#321）。
///
/// **UIKit を使わない。** ImageIO は macOS でも動くので、
/// `swift test` から確かめられる（iOS 専用にすると exclude になり、
/// 誰も検証しないコードになる）。
///
/// 読めないデータはそのまま返す。**握りつぶして空を送らない。**
/// 形式の判断は API 側にあり、そこでエラーになる方が理由が分かる。
func downscaleForEstimate(_ data: Data, maxPixels: Int = estimateMaxPixels) -> Data {
    guard let src = CGImageSourceCreateWithData(data as CFData, nil),
          let props = CGImageSourceCopyPropertiesAtIndex(src, 0, nil) as? [CFString: Any],
          let w = props[kCGImagePropertyPixelWidth] as? Int,
          let h = props[kCGImagePropertyPixelHeight] as? Int
    else { return data }

    // 小さい画像は拡大しない。粗くなるだけで情報は増えない
    guard max(w, h) > maxPixels else { return data }

    let options: [CFString: Any] = [
        kCGImageSourceCreateThumbnailFromImageAlways: true,
        kCGImageSourceThumbnailMaxPixelSize: maxPixels,
        // 撮った向きのまま送る。横倒しの写真は量が読み取りにくい
        kCGImageSourceCreateThumbnailWithTransform: true,
    ]
    guard let thumb = CGImageSourceCreateThumbnailAtIndex(src, 0, options as CFDictionary) else {
        return data
    }

    let out = NSMutableData()
    guard let dest = CGImageDestinationCreateWithData(
        out, UTType.jpeg.identifier as CFString, 1, nil
    ) else { return data }

    // 0.7 は食事の写真で見た目が落ちない下限。これ以上下げると
    // 小さい文字（惣菜のラベルなど）が読めなくなる
    CGImageDestinationAddImage(dest, thumb, [kCGImageDestinationLossyCompressionQuality: 0.7] as CFDictionary)
    guard CGImageDestinationFinalize(dest) else { return data }

    return out as Data
}
