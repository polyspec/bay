// window captures the running window and reports where its rows of type sit, so
// a change to the layout is measured rather than judged by eye.
//
//   swift tools/window.swift out.png            capture only
//   swift tools/window.swift out.png 262 895 205 390   capture and measure
//
// The four numbers are the left, right, top and bottom of the region to measure,
// in the captured image's own points. The output gives each run of pixels that
// holds type, and the distance from one run's centre to the next, which is what
// a reader sees as the rhythm of the rows.

import AppKit
import Foundation

let owner = "baybar"

func windowID() -> Int? {
    let list = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements],
                                          kCGNullWindowID) as? [[String: Any]] ?? []
    var best: (id: Int, area: Double)? = nil
    for w in list {
        let name = w[kCGWindowOwnerName as String] as? String ?? ""
        guard name.lowercased().contains(owner) else { continue }
        let b = w[kCGWindowBounds as String] as? [String: Any] ?? [:]
        let area = (b["Width"] as? Double ?? 0) * (b["Height"] as? Double ?? 0)
        let id = w[kCGWindowNumber as String] as? Int ?? 0
        if best == nil || area > best!.area { best = (id, area) }
    }
    return best?.id
}

guard CommandLine.arguments.count >= 2 else {
    FileHandle.standardError.write("usage: window.swift out.png [x0 x1 y0 y1]\n".data(using: .utf8)!)
    exit(2)
}
let out = CommandLine.arguments[1]

guard let id = windowID() else {
    print("no \(owner) window is open. Start it with -show.")
    exit(1)
}
let shot = Process()
shot.executableURL = URL(fileURLWithPath: "/usr/sbin/screencapture")
shot.arguments = ["-x", "-o", "-l", String(id), out]
try shot.run()
shot.waitUntilExit()
guard shot.terminationStatus == 0 else { exit(shot.terminationStatus) }
print("captured window \(id) to \(out)")

guard CommandLine.arguments.count >= 6,
      let x0 = Int(CommandLine.arguments[2]), let x1 = Int(CommandLine.arguments[3]),
      let y0 = Int(CommandLine.arguments[4]), let y1 = Int(CommandLine.arguments[5]) else { exit(0) }

guard let img = NSImage(contentsOfFile: out), let tiff = img.tiffRepresentation,
      let bmp = NSBitmapImageRep(data: tiff) else { exit(1) }

func lum(_ x: Int, _ y: Int) -> Double {
    guard let c = bmp.colorAt(x: x, y: y) else { return 0 }
    return Double(c.redComponent + c.greenComponent + c.blueComponent) / 3.0
}
// The fill is the value the region holds most of. Type differs from it.
var counts: [Int: Int] = [:]
for y in stride(from: y0, to: y1, by: 2) {
    for x in stride(from: x0, to: x1, by: 3) { counts[Int(lum(x, y) * 255), default: 0] += 1 }
}
let fill = Double(counts.max(by: { $0.value < $1.value })!.key) / 255.0

var runs: [(Int, Int)] = []
var run: (Int, Int)? = nil
for y in y0..<y1 {
    var ink = 0
    for x in x0..<x1 where abs(lum(x, y) - fill) > 0.03 { ink += 1 }
    if ink >= 2 {
        if run == nil { run = (y, y) } else { run!.1 = y }
    } else if let r = run { runs.append(r); run = nil }
}
if let r = run { runs.append(r) }

var previous: Double? = nil
for r in runs {
    let centre = Double(r.0 + r.1) / 2
    var step = ""
    if let p = previous { step = String(format: "  centre +%.1f", centre - p) }
    print("type \(r.0)..\(r.1)  height \(r.1 - r.0 + 1)\(step)")
    previous = centre
}
