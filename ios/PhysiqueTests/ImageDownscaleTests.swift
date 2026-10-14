import CoreGraphics
import Foundation
import ImageIO
import Testing
import UniformTypeIdentifiers

@testable import PhysiqueCore

/// 送る前に画像を縮める（#321）。
///
/// **大きい画像は精度を上げない**（API 側で縮小される）。
/// 送信時間とトークンだけが増えるので、入口で縮める。
@Suite("画像の縮小")
struct ImageDownscaleTests {
    /// 指定した大きさの PNG を作る。**実物の写真を置かない**（ADR-0002）
    private func png(width: Int, height: Int) -> Data {
        let ctx = CGContext(
            data: nil, width: width, height: height,
            bitsPerComponent: 8, bytesPerRow: 0,
            space: CGColorSpaceCreateDeviceRGB(),
            bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
        )!
        ctx.setFillColor(CGColor(red: 0.5, green: 0.5, blue: 0.5, alpha: 1))
        ctx.fill(CGRect(x: 0, y: 0, width: width, height: height))
        let image = ctx.makeImage()!

        let out = NSMutableData()
        let dest = CGImageDestinationCreateWithData(out, UTType.png.identifier as CFString, 1, nil)!
        CGImageDestinationAddImage(dest, image, nil)
        CGImageDestinationFinalize(dest)

        return out as Data
    }

    private func size(of data: Data) -> (Int, Int)? {
        guard let src = CGImageSourceCreateWithData(data as CFData, nil),
              let props = CGImageSourceCopyPropertiesAtIndex(src, 0, nil) as? [CFString: Any],
              let w = props[kCGImagePropertyPixelWidth] as? Int,
              let h = props[kCGImagePropertyPixelHeight] as? Int
        else { return nil }

        return (w, h)
    }

    @Test("長辺を上限まで縮める")
    func shrinksToMax() {
        let got = downscaleForEstimate(png(width: 3000, height: 2000), maxPixels: 1024)
        let (w, h) = size(of: got)!

        #expect(max(w, h) == 1024)
        // **縦横比を保つ。** 歪めると量の推定がずれる
        #expect(abs(Double(w) / Double(h) - 1.5) < 0.01)
    }

    @Test("縦長でも長辺を見る")
    func portrait() {
        let got = downscaleForEstimate(png(width: 1000, height: 2500), maxPixels: 1024)
        let (w, h) = size(of: got)!

        #expect(h == 1024)
        #expect(w < h)
    }

    @Test("小さい画像は拡大しない")
    func doesNotEnlarge() {
        let got = downscaleForEstimate(png(width: 400, height: 300), maxPixels: 1024)
        let (w, h) = size(of: got)!

        #expect(w == 400)
        #expect(h == 300)
    }

    @Test("読めないデータはそのまま返す")
    func passesThroughGarbage() {
        // **握りつぶして空を送らない。** 送って API に判断させる
        let junk = Data([0x00, 0x01, 0x02])

        #expect(downscaleForEstimate(junk, maxPixels: 1024) == junk)
    }

    @Test("縮めると小さくなる")
    func getsSmaller() {
        let big = png(width: 3000, height: 2000)

        #expect(downscaleForEstimate(big, maxPixels: 1024).count < big.count)
    }
}
