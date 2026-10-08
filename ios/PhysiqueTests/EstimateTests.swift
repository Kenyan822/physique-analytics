import Foundation
import Testing

@testable import PhysiqueCore

private let base = URL(string: "http://api.test")!

private let estimated = #"""
{"name":"鶏そぼろ定食","qty":"1人前","kcal":620,"proteinG":42.5,"fatG":18,"carbG":70,
 "confidence":"medium","note":"ご飯は茶碗1杯とみなした","source":"ai_estimated"}
"""#

@Suite("写真から PFC を推定する")
struct EstimateTests {
    @Test("multipart で画像を送る")
    func sendsMultipart() async throws {
        let t = FakeTransport(json: estimated)
        let api = APIClient(baseURL: base, transport: t)

        _ = try await api.estimateMeal(image: Data("fake-jpeg".utf8), note: "鶏むね200g")

        let req = try #require(t.requests.last)
        #expect(req.httpMethod == "POST")
        #expect(req.url?.path == "/v1/meals/estimate")

        let type = try #require(req.value(forHTTPHeaderField: "Content-Type"))
        #expect(type.hasPrefix("multipart/form-data; boundary="))

        let body = String(decoding: try #require(req.httpBody), as: UTF8.self)
        #expect(body.contains(#"name="image""#))
        #expect(body.contains("filename="))
        #expect(body.contains("fake-jpeg"))
        // 量を添えると精度が上がる（openapi.yaml）
        #expect(body.contains(#"name="note""#))
        #expect(body.contains("鶏むね200g"))
    }

    @Test("補足が無ければ note を入れない")
    func omitsNote() async throws {
        let t = FakeTransport(json: estimated)
        let api = APIClient(baseURL: base, transport: t)

        _ = try await api.estimateMeal(image: Data("x".utf8), note: "")

        let body = String(decoding: try #require(t.requests.last?.httpBody), as: UTF8.self)
        #expect(!body.contains(#"name="note""#))
    }

    @Test("推定結果を読める")
    func decodes() async throws {
        let t = FakeTransport(json: estimated)
        let api = APIClient(baseURL: base, transport: t)

        let got = try await api.estimateMeal(image: Data("x".utf8), note: nil)

        #expect(got.name == "鶏そぼろ定食")
        #expect(got.proteinG == 42.5)
        #expect(got.confidence == "medium")
        // **常に ai_estimated。** 記録するときも保つ
        #expect(got.source == .aiEstimated)
    }

    @Test("**鍵が未設定なら 503。黙って失敗しない**")
    func notConfigured() async throws {
        let problem = #"{"type":"about:blank","title":"推定を使えない","status":503,"detail":"API キーが設定されていない"}"#
        let t = FakeTransport(json: problem, status: 503)
        let api = APIClient(baseURL: base, transport: t)

        await #expect(throws: APIError.self) {
            _ = try await api.estimateMeal(image: Data("x".utf8), note: nil)
        }
    }
}

@Suite("推定を下書きに移す")
@MainActor
struct EstimateDraftTests {
    @Test("**そのまま保存しない。** 編集できる下書きになる")
    func toDraft() throws {
        let e = try JSONDecoder().decode(MealEstimate.self, from: Data(estimated.utf8))

        let d = MealDraft(estimate: e)

        #expect(d.name == "鶏そぼろ定食")
        #expect(d.qty == "1人前")
        #expect(d.proteinG == "42.5")
        #expect(d.fatG == "18")
        #expect(d.carbG == "70")
    }

    @Test("値が無い項目は空欄になる")
    func missingValues() throws {
        let json = #"{"name":"わからない","source":"ai_estimated"}"#
        let e = try JSONDecoder().decode(MealEstimate.self, from: Data(json.utf8))

        let d = MealDraft(estimate: e)

        #expect(d.name == "わからない")
        #expect(d.proteinG.isEmpty)
        #expect(d.carbG.isEmpty)
    }
}
