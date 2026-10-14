#if canImport(UIKit)
import SwiftUI
import UIKit

/// その場で撮るためのカメラ（#321）。
///
/// **SwiftUI に標準のカメラが無い**ので `UIImagePickerController` を包む。
/// `PhotosPicker` はライブラリ専用で、撮影はできない。
///
/// `Info.plist` に `NSCameraUsageDescription` が要る。無いと**落ちる**
/// （権限ダイアログが出ないのではなく、プロセスが終了する）。
struct CameraPicker: UIViewControllerRepresentable {
    /// 撮れた画像。閉じただけなら nil
    let onCapture: (Data?) -> Void

    func makeUIViewController(context: Context) -> UIImagePickerController {
        let c = UIImagePickerController()
        c.sourceType = .camera
        c.delegate = context.coordinator

        return c
    }

    func updateUIViewController(_ controller: UIImagePickerController, context: Context) {}

    func makeCoordinator() -> Coordinator { Coordinator(onCapture: onCapture) }

    final class Coordinator: NSObject, UIImagePickerControllerDelegate, UINavigationControllerDelegate {
        private let onCapture: (Data?) -> Void

        init(onCapture: @escaping (Data?) -> Void) { self.onCapture = onCapture }

        func imagePickerController(
            _ picker: UIImagePickerController,
            didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]
        ) {
            let image = info[.originalImage] as? UIImage
            // **縮小はここでしない。** downscaleForEstimate に任せる
            // （ImageIO なので macOS でも動き、テストがある）
            onCapture(image?.jpegData(compressionQuality: 0.9))
        }

        func imagePickerControllerDidCancel(_ picker: UIImagePickerController) {
            onCapture(nil)
        }
    }
}

/// カメラが使えるか。シミュレータでは false。
var cameraIsAvailable: Bool {
    UIImagePickerController.isSourceTypeAvailable(.camera)
}
#endif
