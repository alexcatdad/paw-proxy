import AppKit
import PawProxyNative
import SwiftUI

@main
struct PawProxyApp: App {
    @StateObject private var model = AppModel()

    var body: some Scene {
        MenuBarExtra {
            MenuView(model: model)
        } label: {
            MenuBarLabel(tone: model.presentation.tone, isDevelopment: model.isDevelopment)
        }
        .menuBarExtraStyle(.window)
    }
}

/// Template SF Symbol so macOS tints it for light/dark menu bars; tones
/// other than ok swap to a badged symbol.
struct MenuBarLabel: View {
    var tone: StatusPresentation.Tone
    var isDevelopment: Bool

    var body: some View {
        HStack(spacing: 2) {
            Image(systemName: symbol)
            if isDevelopment { Text("DEV").font(.system(size: 9, weight: .bold)) }
        }
        .accessibilityLabel("Paw Proxy")
    }

    private var symbol: String {
        switch tone {
        case .ok: return "pawprint.fill"
        case .off: return "pawprint"
        case .warning, .update: return "exclamationmark.triangle.fill"
        case .error: return "exclamationmark.octagon.fill"
        }
    }
}
