// Renders the menu-bar template glyph: a paw (one main pad, four toes),
// black on transparent, as Resources/PawProxyTrayIcon.png (18x18 px) and
// PawProxyTrayIcon@2x.png (36x36 px). MenubarKit's TrayIconLoader loads the
// pair as one 18pt template image and adds the attention badge itself.
// Run from macos/: swift Scripts/make-tray-icon.swift
import AppKit

/// The paw in an 18x18 design grid (origin bottom-left). The glyph keeps
/// about 1pt of margin and leaves the bottom-right corner light enough for
/// MenubarKit's badge to sit next to it.
func drawPaw(scale s: CGFloat) {
    NSColor.black.setFill()
    func oval(_ x: CGFloat, _ y: CGFloat, _ w: CGFloat, _ h: CGFloat, rotate degrees: CGFloat = 0) {
        let path = NSBezierPath(ovalIn: NSRect(x: -w / 2, y: -h / 2, width: w, height: h))
        var t = AffineTransform(translationByX: x * s, byY: y * s)
        t.rotate(byDegrees: degrees)
        t.scale(s)
        path.transform(using: t)
        path.fill()
    }
    // Main pad: a rounded, slightly heart-shaped cushion.
    let pad = NSBezierPath()
    pad.move(to: NSPoint(x: 9, y: 10.6))
    pad.curve(to: NSPoint(x: 14.2, y: 4.6), controlPoint1: NSPoint(x: 11.9, y: 10.6), controlPoint2: NSPoint(x: 14.6, y: 7.2))
    pad.curve(to: NSPoint(x: 9, y: 2.6), controlPoint1: NSPoint(x: 13.8, y: 2.4), controlPoint2: NSPoint(x: 11.2, y: 2.9))
    pad.curve(to: NSPoint(x: 3.8, y: 4.6), controlPoint1: NSPoint(x: 6.8, y: 2.9), controlPoint2: NSPoint(x: 4.2, y: 2.4))
    pad.curve(to: NSPoint(x: 9, y: 10.6), controlPoint1: NSPoint(x: 3.4, y: 7.2), controlPoint2: NSPoint(x: 6.1, y: 10.6))
    pad.close()
    pad.transform(using: AffineTransform(scale: s))
    pad.fill()
    // Toes: two inner, two outer, fanned out.
    oval(6.9, 14.0, 3.0, 3.8, rotate: 8)
    oval(11.1, 14.0, 3.0, 3.8, rotate: -8)
    oval(3.0, 10.4, 2.7, 3.4, rotate: 28)
    oval(15.0, 10.4, 2.7, 3.4, rotate: -28)
}

func render(px: Int) -> Data {
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: px, pixelsHigh: px, bitsPerSample: 8,
                               samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
                               colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    NSGraphicsContext.saveGraphicsState()
    let ctx = NSGraphicsContext(bitmapImageRep: rep)!
    ctx.shouldAntialias = true
    NSGraphicsContext.current = ctx
    NSColor.clear.set()
    NSRect(x: 0, y: 0, width: px, height: px).fill()
    drawPaw(scale: CGFloat(px) / 18)
    NSGraphicsContext.restoreGraphicsState()
    return rep.representation(using: .png, properties: [:])!
}

for (px, name) in [(18, "PawProxyTrayIcon.png"), (36, "PawProxyTrayIcon@2x.png")] {
    let url = URL(fileURLWithPath: "Resources/\(name)")
    try render(px: px).write(to: url)
    print("wrote \(url.path)")
}
