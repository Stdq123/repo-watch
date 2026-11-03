import AppKit
import CoreText
import SwiftUI
import UserNotifications

struct Snapshot: Decodable {
    let description: String
    let url: String
    let stars: Int
    let forks: Int
    let issues: Int
    let watchers: Int?
    let language: String?
    let license: String?
    let createdAt: String?
    let pushedAt: String
    let release: String
    let releaseUrl: String
    let releaseNotes: String?
    let checkedAt: String
}
struct Repository: Decodable, Identifiable {
    let id: String
    let platform: String
    let owner: String
    let repo: String
    let tags: [String]?
    let localPath: String?
    let snapshot: Snapshot?
    let error: String
    let change: String
}

final class Store: ObservableObject {
    @Published var repositories: [Repository] = []
    @Published var error: String?
    private var process: Process?
    private let input = Pipe()
    private let output = Pipe()
    private var pending: [Int: (Any?) -> Void] = [:]
    private var nextID = 0
    private var buffer = Data()
    private var timer: Timer?

    init() {
        guard let helper = Bundle.main.url(forAuxiliaryExecutable: "repowatch-helper") else {
            error = "应用缺少监控服务，请重新构建。"; return
        }
        let child = Process()
        child.executableURL = helper
        child.standardInput = input
        child.standardOutput = output
        child.standardError = FileHandle.standardError
        output.fileHandleForReading.readabilityHandler = { [weak self] handle in
            let data = handle.availableData
            if data.isEmpty { handle.readabilityHandler = nil; return }
            DispatchQueue.main.async { self?.receive(data) }
        }
        child.terminationHandler = { [weak self] _ in
            DispatchQueue.main.async {
                self?.timer?.invalidate()
                if self?.error == nil { self?.error = "监控服务已退出，请重新打开应用。" }
            }
        }
        do {
            try child.run()
            process = child
            refreshList()
            timer = Timer.scheduledTimer(withTimeInterval: 2, repeats: true) { [weak self] _ in self?.refreshList() }
        } catch { self.error = "无法启动监控服务：\(error.localizedDescription)" }
    }

    func send(_ method: String, _ values: [String: Any] = [:], completion: ((Any?) -> Void)? = nil) {
        guard process?.isRunning == true else { error = "监控服务未运行。"; return }
        nextID += 1
        var request = values
        request["id"] = nextID
        request["method"] = method
        if let completion = completion { pending[nextID] = completion }
        do {
            var data = try JSONSerialization.data(withJSONObject: request)
            data.append(10)
            try input.fileHandleForWriting.write(contentsOf: data)
        } catch {
            pending.removeValue(forKey: nextID)
            self.error = "无法与监控服务通信：\(error.localizedDescription)"
        }
    }

    private func receive(_ data: Data) {
        buffer.append(data)
        while let end = buffer.firstIndex(of: 10) {
            let line = buffer.prefix(upTo: end)
            buffer.removeSubrange(...end)
            guard let object = try? JSONSerialization.jsonObject(with: line) as? [String: Any] else { continue }
            if object["event"] as? String == "fatal" { error = object["body"] as? String ?? "监控服务启动失败。"; continue }
            if object["event"] as? String == "notification" {
                let content = UNMutableNotificationContent()
                content.title = object["title"] as? String ?? "Repo Watch"
                content.body = object["body"] as? String ?? ""
                UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil))
                continue
            }
            guard let id = object["id"] as? Int else { continue }
            let callback = pending.removeValue(forKey: id)
            if let message = object["error"] as? String, !message.isEmpty { error = message; continue }
            callback?(object["result"])
        }
    }
    func refreshList() {
        send("list") { [weak self] value in
            guard let value = value, let data = try? JSONSerialization.data(withJSONObject: value),
                  let repos = try? JSONDecoder().decode([Repository].self, from: data) else { return }
            self?.repositories = repos
        }
    }
    func mutate(_ method: String, _ values: [String: Any]) { send(method, values) { [weak self] _ in self?.refreshList() } }
    func stop() {
        timer?.invalidate()
        output.fileHandleForReading.readabilityHandler = nil
        try? input.fileHandleForWriting.close()
    }
}

func splitTags(_ text: String) -> [String] { text.components(separatedBy: CharacterSet(charactersIn: ",，")).map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }.filter { !$0.isEmpty } }

struct AddView: View {
    @ObservedObject var store: Store
    @Environment(\.dismiss) private var dismiss
    @State private var url = ""
    @State private var platform = "github"
    @State private var owner = ""
    @State private var repo = ""
    @State private var baseURL = ""
    @State private var tags = ""
    @State private var token = ""
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("添加仓库").font(.title2.bold())
            TextField("仓库链接（可选，填写后自动识别）", text: $url)
            Picker("平台", selection: $platform) { Text("GitHub").tag("github"); Text("Codeberg").tag("codeberg"); Text("Gitea（自建）").tag("gitea") }
            HStack { TextField("Owner", text: $owner); TextField("Repository", text: $repo) }
            if platform == "gitea" { TextField("Gitea 实例地址", text: $baseURL) }
            TextField("标签（逗号分隔）", text: $tags)
            SecureField("私有仓库访问令牌（可选）", text: $token)
            Text("令牌保存在 macOS 钥匙串中，只通过 HTTPS 发送。").font(.caption).foregroundColor(.secondary)
            HStack { Spacer(); Button("取消") { dismiss() }; Button("添加", action: addRepository).keyboardShortcut(.defaultAction) }
        }.padding(24).frame(width: 480)
        .alert("操作失败", isPresented: Binding(get: { store.error != nil }, set: { if !$0 { store.error = nil } })) { Button("确定") { store.error = nil } } message: { Text(store.error ?? "") }
    }
    private func addRepository() {
        var address = url.trimmingCharacters(in: .whitespacesAndNewlines)
        if address.isEmpty {
            let ownerName = owner.trimmingCharacters(in: .whitespacesAndNewlines)
            let repoName = repo.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !ownerName.isEmpty, !repoName.isEmpty, !ownerName.contains("/"), !repoName.contains("/") else { store.error = "请填写有效的 Owner 和 Repository。"; return }
            let base: String
            if platform == "github" { base = "https://github.com" }
            else if platform == "codeberg" { base = "https://codeberg.org" }
            else { base = baseURL.trimmingCharacters(in: .whitespacesAndNewlines).trimmingCharacters(in: CharacterSet(charactersIn: "/")) }
            guard !base.isEmpty else { store.error = "请填写 Gitea 实例地址。"; return }
            address = "\(base)/\(ownerName)/\(repoName)"
        }
        store.send("add", ["url": address, "tags": splitTags(tags), "token": token]) { _ in self.store.refreshList(); self.dismiss() }
    }
}
struct EditView: View {
    @ObservedObject var store: Store
    let repo: Repository
    @Environment(\.dismiss) private var dismiss
    @State private var tags = ""
    @State private var token = ""
    @State private var path = ""
    @State private var clearingToken = false
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("编辑 \(repo.owner)/\(repo.repo)").font(.title2.bold())
            TextField("标签（逗号分隔）", text: $tags)
            HStack { TextField("本地仓库目录", text: $path); Button("选择") { let panel = NSOpenPanel(); panel.canChooseDirectories = true; panel.canChooseFiles = false; if panel.runModal() == .OK { path = panel.url?.path ?? path } } }
            SecureField("新令牌（留空保持不变）", text: $token)
            HStack {
                Button("清除令牌") { clearingToken = true }
                Spacer(); Button("取消") { dismiss() }
                Button("保存") {
                    store.send("tags", ["repositoryID": repo.id, "tags": splitTags(tags)]) { _ in
                        self.store.send("path", ["repositoryID": self.repo.id, "path": self.path]) { _ in
                            if self.token.isEmpty { self.store.refreshList(); self.dismiss() }
                            else { self.store.send("credential", ["repositoryID": self.repo.id, "token": self.token]) { _ in self.store.refreshList(); self.dismiss() } }
                        }
                    }
                }.keyboardShortcut(.defaultAction)
            }
        }.padding(24).frame(width: 480).onAppear { tags = (repo.tags ?? []).joined(separator: ", "); path = repo.localPath ?? "" }
        .confirmationDialog("确定清除该仓库的本机令牌？", isPresented: $clearingToken) {
            Button("清除令牌", role: .destructive) { store.mutate("credential", ["repositoryID": repo.id, "token": ""]) }
            Button("取消", role: .cancel) { }
        }
        .alert("操作失败", isPresented: Binding(get: { store.error != nil }, set: { if !$0 { store.error = nil } })) { Button("确定") { store.error = nil } } message: { Text(store.error ?? "") }
    }
}

@main
final class AppDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate {
    var window: NSWindow!
    var status: NSStatusItem!
    var store: Store!
    func applicationDidFinishLaunching(_ notification: Notification) {
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound]) { _, _ in }
        for fontName in ["Inter-Regular", "Inter-Bold"] {
            if let url = Bundle.main.url(forResource: fontName, withExtension: "ttf", subdirectory: "fonts") {
                CTFontManagerRegisterFontsForURL(url as CFURL, .process, nil)
            }
        }
        store = Store()
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1440, height: 900), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        window.title = "Repo Watch"; window.center(); window.delegate = self; window.isReleasedWhenClosed = false
        window.contentView = NSHostingView(rootView: MainView(store: store)); showWindow()
        let mainMenu = NSMenu(); let appMenuItem = NSMenuItem(); mainMenu.addItem(appMenuItem)
        let appMenu = NSMenu(); appMenu.addItem(withTitle: "退出 Repo Watch", action: #selector(quit), keyEquivalent: "q"); appMenuItem.submenu = appMenu; NSApp.mainMenu = mainMenu
        status = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        status.button?.title = "◉"
        let menu = NSMenu(); menu.addItem(withTitle: "显示 Repo Watch", action: #selector(showWindow), keyEquivalent: ""); menu.addItem(withTitle: "立即刷新", action: #selector(refresh), keyEquivalent: ""); menu.addItem(.separator()); menu.addItem(withTitle: "退出 Repo Watch", action: #selector(quit), keyEquivalent: "q"); status.menu = menu
    }
    func windowShouldClose(_ sender: NSWindow) -> Bool { sender.orderOut(nil); return false }
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows: Bool) -> Bool { showWindow(); return true }
    func applicationWillTerminate(_ notification: Notification) { store.stop() }
    @objc func showWindow() { window.makeKeyAndOrderFront(nil); NSApp.activate(ignoringOtherApps: true) }
    @objc func refresh() { store.send("refresh") }
    @objc func quit() { NSApp.terminate(nil) }
    static func main() { let app = NSApplication.shared; let delegate = AppDelegate(); app.delegate = delegate; app.setActivationPolicy(.regular); app.run(); _ = delegate }
}
