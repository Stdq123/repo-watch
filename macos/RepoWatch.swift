import AppKit
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

struct MainView: View {
    @ObservedObject var store: Store
    @State private var search = ""
    @State private var tag = ""
    @State private var order = "name"
    @State private var adding = false
    @State private var editing: Repository?
    @State private var removing: Repository?
    @State private var pinned = false
    private var tags: [String] { Array(Set(store.repositories.flatMap { $0.tags ?? [] })).sorted() }
    private var filtered: [Repository] {
        store.repositories.filter { repo in
            let text = "\(repo.owner)/\(repo.repo) \(repo.platform) \((repo.tags ?? []).joined(separator: " ")) \(repo.snapshot?.description ?? "")"
            return (tag.isEmpty || (repo.tags ?? []).contains(tag)) && (search.isEmpty || text.localizedCaseInsensitiveContains(search))
        }.sorted { a, b in
            if order == "stars", (a.snapshot?.stars ?? -1) != (b.snapshot?.stars ?? -1) { return (a.snapshot?.stars ?? -1) > (b.snapshot?.stars ?? -1) }
            if order == "forks", (a.snapshot?.forks ?? -1) != (b.snapshot?.forks ?? -1) { return (a.snapshot?.forks ?? -1) > (b.snapshot?.forks ?? -1) }
            if order == "updated", (a.snapshot?.pushedAt ?? "") != (b.snapshot?.pushedAt ?? "") { return (a.snapshot?.pushedAt ?? "") > (b.snapshot?.pushedAt ?? "") }
            return "\(a.owner)/\(a.repo)".localizedCaseInsensitiveCompare("\(b.owner)/\(b.repo)") == .orderedAscending
        }
    }
    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack {
                VStack(alignment: .leading) { Text("Repo Watch").font(.largeTitle.bold()); Text("仓库动态，一目了然").foregroundColor(.secondary) }
                Spacer()
                Toggle("置顶", isOn: $pinned).toggleStyle(.checkbox).onChange(of: pinned) { value in NSApp.windows.first(where: { $0.title == "Repo Watch" })?.level = value ? .floating : .normal }
                Button("刷新") { store.send("refresh") }.keyboardShortcut("r")
                Button("添加仓库") { adding = true }.keyboardShortcut("n")
            }
            HStack {
                TextField("搜索仓库、平台或标签", text: $search).textFieldStyle(.roundedBorder)
                Picker("标签", selection: $tag) { Text("全部标签").tag(""); ForEach(tags, id: \.self) { Text($0).tag($0) } }.frame(width: 160)
                Picker("排序", selection: $order) { Text("名称").tag("name"); Text("Stars").tag("stars"); Text("Forks").tag("forks"); Text("更新时间").tag("updated") }.frame(width: 140)
            }
            if filtered.isEmpty { Spacer(); Text("暂无匹配的仓库").foregroundColor(.secondary).frame(maxWidth: .infinity); Spacer() }
            else {
                ScrollView {
                    LazyVStack(spacing: 10) {
                        ForEach(filtered) { repo in
                            VStack(alignment: .leading, spacing: 8) {
                                HStack {
                                    Text("\(repo.owner)/\(repo.repo)").font(.headline)
                                    Text(repo.platform.uppercased()).font(.caption).foregroundColor(.secondary)
                                    Spacer()
                                    Button("打开") { if let raw = repo.snapshot?.url, let url = URL(string: raw), ["https", "http"].contains(url.scheme?.lowercased() ?? "") { NSWorkspace.shared.open(url) } }
                                    Button("终端") { openTerminal(repo) }
                                    Button("编辑") { editing = repo }
                                    Button("删除") { removing = repo }
                                }
                                if let snapshot = repo.snapshot {
                                    Text(snapshot.description.isEmpty ? "暂无描述" : snapshot.description).foregroundColor(.secondary)
                                    Text("★ \(snapshot.stars)     Forks \(snapshot.forks)     Issues \(snapshot.issues)     关注 \(snapshot.watchers.map { String($0) } ?? "—")     \(snapshot.language ?? "")     \(snapshot.license ?? "")")
                                    HStack {
                                        Text("最新版本：\(snapshot.release.isEmpty ? "暂无" : snapshot.release)")
                                        if let url = URL(string: snapshot.releaseUrl), ["https", "http"].contains(url.scheme?.lowercased() ?? "") { Link("查看发行版", destination: url) }
                                        Text("更新：\(snapshot.pushedAt.isEmpty ? "暂无" : snapshot.pushedAt)")
                                        Text("检查：\(snapshot.checkedAt)")
                                    }.font(.caption).foregroundColor(.secondary)
                                } else { Text("等待首次刷新").foregroundColor(.secondary) }
                                if !(repo.tags ?? []).isEmpty { Text((repo.tags ?? []).joined(separator: "  ·  ")).font(.caption).foregroundColor(.accentColor) }
                                if !repo.change.isEmpty { Text(repo.change).font(.caption).foregroundColor(.secondary) }
                                if !repo.error.isEmpty { Text(repo.error).font(.caption).foregroundColor(.red) }
                            }.padding(14).frame(maxWidth: .infinity, alignment: .leading).background(Color(nsColor: .controlBackgroundColor)).cornerRadius(10)
                        }
                    }
                }
            }
        }.padding(20).frame(minWidth: 800, minHeight: 520)
        .sheet(isPresented: $adding) { AddView(store: store) }
        .sheet(item: $editing) { repo in EditView(store: store, repo: repo) }
        .alert(item: $removing) { repo in Alert(title: Text("删除 \(repo.owner)/\(repo.repo)？"), message: Text("本机保存的令牌也会被删除。"), primaryButton: .destructive(Text("删除")) { store.mutate("remove", ["repositoryID": repo.id]) }, secondaryButton: .cancel()) }
        .alert("操作失败", isPresented: Binding(get: { store.error != nil }, set: { if !$0 { store.error = nil } })) { Button("确定") { store.error = nil } } message: { Text(store.error ?? "") }
    }
    private func openTerminal(_ repo: Repository) {
        if let path = repo.localPath, !path.isEmpty { launchTerminal(path); return }
        let panel = NSOpenPanel(); panel.canChooseDirectories = true; panel.canChooseFiles = false
        if panel.runModal() == .OK, let path = panel.url?.path {
            store.send("path", ["repositoryID": repo.id, "path": path]) { _ in self.launchTerminal(path); self.store.refreshList() }
        }
    }
    private func launchTerminal(_ path: String) {
        let task = Process(); task.executableURL = URL(fileURLWithPath: "/usr/bin/open"); task.arguments = ["-a", "Terminal", path]
        do { try task.run() } catch { store.error = "无法打开终端：\(error.localizedDescription)" }
    }
}

func splitTags(_ text: String) -> [String] { text.components(separatedBy: CharacterSet(charactersIn: ",，")).map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }.filter { !$0.isEmpty } }

struct AddView: View {
    @ObservedObject var store: Store
    @Environment(\.dismiss) private var dismiss
    @State private var url = ""
    @State private var tags = ""
    @State private var token = ""
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("添加仓库").font(.title2.bold())
            TextField("仓库首页链接，例如 https://github.com/owner/repo", text: $url)
            TextField("标签（逗号分隔）", text: $tags)
            SecureField("私有仓库访问令牌（可选）", text: $token)
            Text("令牌保存在 macOS 钥匙串中，只通过 HTTPS 发送。").font(.caption).foregroundColor(.secondary)
            HStack { Spacer(); Button("取消") { dismiss() }; Button("添加") { store.send("add", ["url": url, "tags": splitTags(tags), "token": token]) { _ in self.store.refreshList(); self.dismiss() } }.keyboardShortcut(.defaultAction) }
        }.padding(24).frame(width: 480)
        .alert("操作失败", isPresented: Binding(get: { store.error != nil }, set: { if !$0 { store.error = nil } })) { Button("确定") { store.error = nil } } message: { Text(store.error ?? "") }
    }
}
struct EditView: View {
    @ObservedObject var store: Store
    let repo: Repository
    @Environment(\.dismiss) private var dismiss
    @State private var tags = ""
    @State private var token = ""
    @State private var path = ""
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("编辑 \(repo.owner)/\(repo.repo)").font(.title2.bold())
            TextField("标签（逗号分隔）", text: $tags)
            HStack { TextField("本地仓库目录", text: $path); Button("选择") { let panel = NSOpenPanel(); panel.canChooseDirectories = true; panel.canChooseFiles = false; if panel.runModal() == .OK { path = panel.url?.path ?? path } } }
            SecureField("新令牌（留空保持不变）", text: $token)
            HStack {
                Button("清除令牌") { store.mutate("credential", ["repositoryID": repo.id, "token": ""]) }
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
        .alert("操作失败", isPresented: Binding(get: { store.error != nil }, set: { if !$0 { store.error = nil } })) { Button("确定") { store.error = nil } } message: { Text(store.error ?? "") }
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate {
    var window: NSWindow!
    var status: NSStatusItem!
    var store: Store!
    func applicationDidFinishLaunching(_ notification: Notification) {
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound]) { _, _ in }
        store = Store()
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1080, height: 720), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
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
AppDelegate.main()
