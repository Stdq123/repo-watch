import AppKit
import SwiftUI

private enum WatchStyle {
    static let background = Color(red: 248/255, green: 249/255, blue: 249/255)
    static let surface = Color.white
    static let ink = Color(red: 18/255, green: 24/255, blue: 36/255)
    static let muted = Color(red: 99/255, green: 110/255, blue: 121/255)
    static let green = Color(red: 26/255, green: 91/255, blue: 55/255)
    static let selected = Color(red: 234/255, green: 245/255, blue: 237/255)
    static let border = Color(red: 232/255, green: 235/255, blue: 236/255)
    static let header = Color(red: 245/255, green: 247/255, blue: 247/255)
}

private func watchFont(_ size: CGFloat, bold: Bool = false) -> Font {
    .custom(bold ? "Inter-Bold" : "Inter-Regular", size: size)
}

private struct WatchIcon: View {
    let name: String
    var size: CGFloat = 16
    var color: Color = WatchStyle.ink
    var family: String = "lucide"
    var body: some View {
        if let url = Bundle.main.url(forResource: name, withExtension: "png", subdirectory: "icons/\(family)"),
           let icon = NSImage(contentsOf: url) {
            Image(nsImage: icon).renderingMode(.template).resizable().interpolation(.high)
                .frame(width: size, height: size).foregroundColor(color).accessibilityHidden(true)
        } else {
            Image(systemName: "circle").resizable().frame(width: size, height: size)
                .foregroundColor(color).accessibilityHidden(true)
        }
    }
}

// AppKit 菜单项，持有闭包，避免 SwiftUI Menu 在 borderlessButton 样式下丢弃自定义标签样式。
private final class MenuActionItem: NSMenuItem {
    private let handler: () -> Void
    init(_ title: String, checked: Bool = false, handler: @escaping () -> Void) {
        self.handler = handler
        super.init(title: title, action: #selector(fire), keyEquivalent: "")
        target = self
        state = checked ? .on : .off
    }
    required init(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }
    @objc private func fire() { handler() }
}

private func showPopupMenu(_ items: [(title: String, checked: Bool, handler: () -> Void)]) {
    let menu = NSMenu()
    for item in items {
        menu.addItem(MenuActionItem(item.title, checked: item.checked, handler: item.handler))
    }
    menu.popUp(positioning: nil, at: NSEvent.mouseLocation, in: nil)
}

private struct WatchButtonStyle: ButtonStyle {
    var primary = false
    func makeBody(configuration: Configuration) -> some View {
        configuration.label.font(watchFont(13, bold: primary))
            .foregroundColor(primary ? .white : WatchStyle.ink)
            .padding(.horizontal, 12).frame(height: 36)
            .background(primary ? WatchStyle.green : WatchStyle.surface)
            .overlay(RoundedRectangle(cornerRadius: 5).stroke(primary ? WatchStyle.green : WatchStyle.border))
            .cornerRadius(5).opacity(configuration.isPressed ? 0.75 : 1)
    }
}

private struct TagPill: View {
    let text: String
    private var colors: (Color, Color) {
        var hash: UInt32 = 2166136261
        for scalar in text.unicodeScalars { hash = (hash ^ scalar.value) &* 16777619 }
        let bg: [Color] = [Color(red: 219/255, green: 239/255, blue: 220/255), Color(red: 218/255, green: 234/255, blue: 251/255), Color(red: 237/255, green: 228/255, blue: 250/255), Color(red: 1, green: 244/255, blue: 205/255)]
        let fg: [Color] = [Color(red: 38/255, green: 105/255, blue: 54/255), Color(red: 36/255, green: 79/255, blue: 133/255), Color(red: 89/255, green: 61/255, blue: 136/255), Color(red: 116/255, green: 88/255, blue: 24/255)]
        let index = Int(hash % 4)
        return (bg[index], fg[index])
    }
    var body: some View {
        Text(text).font(watchFont(11)).lineLimit(1).padding(.horizontal, 8).padding(.vertical, 4)
            .foregroundColor(colors.1).background(colors.0).cornerRadius(5)
    }
}

private enum SortOrder: String, CaseIterable {
    case updated = "最近更新", name = "仓库名称", stars = "Stars", forks = "Forks"
}

struct MainView: View {
    @ObservedObject var store: Store
    @State private var search = ""
    @State private var selectedTag = ""
    @State private var order: SortOrder = .updated
    @State private var reversed = false
    @State private var selectedID: String?
    @State private var adding = false
    @State private var editing: Repository?
    @State private var removing: Repository?
    @State private var pinned = false

    private var tags: [String] { Array(Set(store.repositories.flatMap { $0.tags ?? [] })).sorted() }
    private var filtered: [Repository] {
        let results = store.repositories.filter { repo in
            let text = "\(repo.owner)/\(repo.repo) \(repo.platform) \((repo.tags ?? []).joined(separator: " ")) \(repo.snapshot?.description ?? "")"
            return (selectedTag.isEmpty || (repo.tags ?? []).contains(selectedTag)) && (search.isEmpty || text.localizedCaseInsensitiveContains(search))
        }.sorted { a, b in
            let comparison: ComparisonResult
            switch order {
            case .name: comparison = "\(a.owner)/\(a.repo)".localizedCaseInsensitiveCompare("\(b.owner)/\(b.repo)")
            case .stars: comparison = compareNumber(a.snapshot?.stars, b.snapshot?.stars)
            case .forks: comparison = compareNumber(a.snapshot?.forks, b.snapshot?.forks)
            case .updated: comparison = compareDate(a.snapshot?.pushedAt ?? "", b.snapshot?.pushedAt ?? "")
            }
            if comparison == .orderedSame { return "\(a.owner)/\(a.repo)".localizedCaseInsensitiveCompare("\(b.owner)/\(b.repo)") == .orderedAscending }
            return reversed ? comparison == .orderedDescending : comparison == .orderedAscending
        }
        return results
    }
    private var selected: Repository? { filtered.first { $0.id == selectedID } }
    private func compareNumber(_ a: Int?, _ b: Int?) -> ComparisonResult {
        let left = a ?? -1, right = b ?? -1
        return left == right ? .orderedSame : (left > right ? .orderedAscending : .orderedDescending)
    }
    private func compareDate(_ a: String, _ b: String) -> ComparisonResult {
        let left = parseDate(a) ?? .distantPast, right = parseDate(b) ?? .distantPast
        return left == right ? .orderedSame : (left > right ? .orderedAscending : .orderedDescending)
    }
    private func chooseOrder(_ value: SortOrder) {
        if order == value { reversed.toggle() } else { order = value; reversed = false }
    }
    private func keepSelection() {
        if !filtered.contains(where: { $0.id == selectedID }) { selectedID = filtered.first?.id }
    }

    var body: some View {
        VStack(spacing: 0) {
            topBar
            HStack(alignment: .top, spacing: 0) {
                VStack(spacing: 0) {
                    HStack(spacing: 0) { sidebar; repositoryTable }
                    footer
                }
                Rectangle().fill(WatchStyle.border).frame(width: 1)
                inspector.frame(width: 320).background(WatchStyle.surface)
            }
        }
        .background(WatchStyle.background).foregroundColor(WatchStyle.ink)
        .environment(\.colorScheme, .light)
        .frame(minWidth: 1050, minHeight: 650)
        .onAppear(perform: keepSelection)
        .onChange(of: filtered.map { $0.id }) { _ in keepSelection() }
        .onChange(of: pinned) { value in NSApp.windows.first(where: { $0.title == "Repo Watch" })?.level = value ? .floating : .normal }
        .sheet(isPresented: $adding) { AddView(store: store) }
        .sheet(item: $editing) { repo in EditView(store: store, repo: repo) }
        .alert(item: $removing) { repo in
            Alert(title: Text("移除 \(repo.owner)/\(repo.repo)？"), message: Text("该仓库的本机令牌也会被删除。"), primaryButton: .destructive(Text("移除")) { store.mutate("remove", ["repositoryID": repo.id]) }, secondaryButton: .cancel())
        }
        .alert("操作失败", isPresented: Binding(get: { store.error != nil }, set: { if !$0 { store.error = nil } })) { Button("确定") { store.error = nil } } message: { Text(store.error ?? "") }
    }

    private var topBar: some View {
        HStack(spacing: 10) {
            WatchIcon(name: "binoculars", size: 20, color: .white).frame(width: 40, height: 40).background(WatchStyle.green).cornerRadius(6)
            Text("Repo Watch").font(watchFont(20, bold: true))
            Spacer()
            Button(action: { adding = true }) { Label { Text("添加仓库") } icon: { WatchIcon(name: "plus", color: .white) } }.buttonStyle(WatchButtonStyle(primary: true)).keyboardShortcut("n")
            Button(action: { store.send("refresh") }) { Label { Text("刷新") } icon: { WatchIcon(name: "refresh-cw") } }.buttonStyle(WatchButtonStyle()).keyboardShortcut("r")
            Menu { Toggle("窗口置顶", isOn: $pinned) } label: { Image(systemName: "gearshape").font(.system(size: 15)).frame(width: 32, height: 36) }.menuStyle(.borderlessButton).fixedSize().help("设置")
        }.padding(.horizontal, 16).frame(height: 58)
    }

    private var sidebar: some View {
        ScrollView {
            VStack(spacing: 3) {
                sidebarRow("全部仓库", tag: "", count: store.repositories.count, icon: "layout-grid")
                ForEach(tags, id: \.self) { tag in
                    sidebarRow(tag, tag: tag, count: store.repositories.filter { ($0.tags ?? []).contains(tag) }.count, icon: "tag")
                }
            }.padding(.horizontal, 10).padding(.top, 16)
        }.frame(width: 220).background(WatchStyle.background)
    }
    private func sidebarRow(_ title: String, tag: String, count: Int, icon: String) -> some View {
        let active = selectedTag == tag
        return Button(action: { selectedTag = tag }) {
            HStack(spacing: 8) {
                WatchIcon(name: icon, color: active ? WatchStyle.green : WatchStyle.muted)
                Text(title).lineLimit(1); Spacer(); Text("\(count)").foregroundColor(WatchStyle.muted)
            }.font(watchFont(13)).padding(.horizontal, 10).frame(height: 38)
                .background(active ? WatchStyle.selected : .clear).cornerRadius(5)
                .overlay(alignment: .leading) { if active { RoundedRectangle(cornerRadius: 2).fill(WatchStyle.green).frame(width: 3).padding(.vertical, 4) } }
        }.buttonStyle(.plain).accessibilityLabel("\(title)，\(count) 个仓库")
    }

    private var repositoryTable: some View {
        VStack(spacing: 8) {
            HStack(spacing: 8) {
                HStack(spacing: 8) {
                    WatchIcon(name: "search", size: 15, color: WatchStyle.muted)
                    TextField("搜索仓库、平台或标签", text: $search).textFieldStyle(.plain).font(watchFont(13))
                }.padding(.horizontal, 10).frame(height: 38).background(WatchStyle.surface).overlay(RoundedRectangle(cornerRadius: 7).stroke(WatchStyle.border))
                Button(action: {
                    showPopupMenu(SortOrder.allCases.map { item in (item.rawValue, item == order, { order = item; reversed = false }) })
                }) {
                    HStack(spacing: 6) {
                        Text("排序：\(order.rawValue)").lineLimit(1)
                        Spacer(minLength: 4)
                        WatchIcon(name: "chevron-down", size: 12, color: WatchStyle.muted)
                    }.font(watchFont(12)).padding(.horizontal, 10).frame(width: 150, height: 38)
                        .background(WatchStyle.surface).overlay(RoundedRectangle(cornerRadius: 5).stroke(WatchStyle.border)).cornerRadius(5)
                }.buttonStyle(.plain).help("选择排序方式")
                Button(action: { reversed.toggle() }) { WatchIcon(name: (order == .name) != reversed ? "arrow-up" : "arrow-down").frame(width: 38, height: 38).background(WatchStyle.surface).overlay(RoundedRectangle(cornerRadius: 5).stroke(WatchStyle.border)) }.buttonStyle(.plain).help("切换排序方向")
            }
            tableHeader
            if filtered.isEmpty {
                VStack { Spacer(); Text(store.repositories.isEmpty ? "还没有仓库，点击右上角添加。" : "没有匹配的仓库，请调整搜索或标签筛选。").foregroundColor(WatchStyle.muted).font(watchFont(13)); Spacer() }.frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                ScrollView {
                    LazyVStack(spacing: 0) {
                        ForEach(filtered) { repo in
                            Button(action: { selectedID = repo.id }) { RepositoryRow(repo: repo, selected: repo.id == selectedID) }
                                .buttonStyle(.plain)
                                .simultaneousGesture(TapGesture(count: 2).onEnded { open(repo.snapshot?.url) })
                                .contextMenu { Button("打开仓库") { open(repo.snapshot?.url) }; Button("编辑仓库") { editing = repo }; Button("移除仓库") { removing = repo } }
                        }
                    }
                }
            }
        }.padding(.horizontal, 14).padding(.top, 18).padding(.bottom, 8).frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(WatchStyle.surface).overlay(RoundedRectangle(cornerRadius: 6).stroke(WatchStyle.border))
    }
    private var tableHeader: some View {
        GeometryReader { geometry in
            let widths = columnWidths(geometry.size.width)
            HStack(spacing: 0) {
                headerButton("仓库", width: widths[0], sort: .name)
                Text("平台").frame(width: widths[1], alignment: .leading)
                Text("标签").frame(width: widths[2], alignment: .leading)
                if widths[3] > 0 { headerButton("Stars", width: widths[3], sort: .stars); headerButton("Forks", width: widths[4], sort: .forks); headerButton("最近更新", width: widths[5], sort: .updated) }
            }.font(watchFont(12, bold: true)).padding(.leading, 8).frame(height: 42).background(WatchStyle.header).cornerRadius(6)
        }.frame(height: 42)
    }
    private func headerButton(_ title: String, width: CGFloat, sort: SortOrder) -> some View {
        Button(action: { chooseOrder(sort) }) {
            HStack(spacing: 3) { Text(title); if order == sort { WatchIcon(name: (sort == .name) != reversed ? "arrow-up" : "arrow-down", size: 11, color: WatchStyle.muted) } }.frame(width: width, alignment: .leading)
        }.buttonStyle(.plain)
    }
    private var footer: some View {
        HStack(spacing: 7) {
            Circle().fill(WatchStyle.green).frame(width: 6, height: 6)
            Text("正在关注 \(store.repositories.count) 个仓库"); Text("│").foregroundColor(WatchStyle.border)
            Text(store.repositories.compactMap { $0.snapshot?.checkedAt }.max().map { "最近检查：\(relativeDate($0))" } ?? "尚未刷新")
            Spacer(); Text("✓  持续监控中")
        }.font(watchFont(11)).foregroundColor(WatchStyle.muted).padding(.horizontal, 16).frame(height: 34)
    }

    @ViewBuilder private var inspector: some View {
        if let repo = selected {
            RepositoryInspector(repo: repo, open: open, terminal: { openTerminal(repo) }, edit: { editing = repo }, remove: { removing = repo })
        } else {
            VStack(alignment: .leading, spacing: 16) {
                HStack(spacing: 12) { WatchIcon(name: "book-open", size: 26, color: WatchStyle.green).frame(width: 54, height: 54).background(WatchStyle.selected).cornerRadius(8); VStack(alignment: .leading) { Text("选择一个仓库").font(watchFont(17, bold: true)); Text("查看仓库信息和动态").font(watchFont(11)).foregroundColor(WatchStyle.muted) } }
                Text(store.repositories.isEmpty ? "还没有仓库，点击右上角添加。" : "没有匹配的仓库，请调整搜索或标签筛选。").foregroundColor(WatchStyle.muted).font(watchFont(12))
                Spacer()
            }.padding(20).frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        }
    }
    private func open(_ raw: String?) {
        guard let raw = raw, let url = URL(string: raw), ["https", "http"].contains(url.scheme?.lowercased() ?? ""), url.host != nil else { store.error = "暂无可打开的链接，请先刷新仓库。"; return }
        NSWorkspace.shared.open(url)
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

private func columnWidths(_ width: CGFloat) -> [CGFloat] {
    let w = max(0, width - 8)
    if w < 600 { return [w * 0.45, w * 0.19, w * 0.36, 0, 0, 0] }
    return [w * 0.30, w * 0.14, w * 0.17, w * 0.11, w * 0.10, w * 0.18]
}
private func compactCount(_ value: Int?) -> String {
    guard let value = value else { return "—" }
    if value < 1000 { return String(value) }
    let n = value >= 1000000 ? Double(value) / 1000000 : Double(value) / 1000
    let number = String(format: "%.1f", n).replacingOccurrences(of: ".0", with: "")
    return number + (value >= 1000000 ? "m" : "k")
}
private func parseDate(_ value: String) -> Date? {
    let parser = ISO8601DateFormatter()
    if let date = parser.date(from: value) { return date }
    parser.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return parser.date(from: value)
}
private func relativeDate(_ value: String) -> String {
    guard let date = parseDate(value) else { return "暂无记录" }
    let age = max(0, Int(Date().timeIntervalSince(date)))
    if age < 60 { return "刚刚" }
    if age < 3600 { return "\(age / 60) 分钟前" }
    if age < 86400 { return "\(age / 3600) 小时前" }
    if age < 30 * 86400 { return "\(age / 86400) 天前" }
    let formatter = DateFormatter(); formatter.dateFormat = "yyyy-MM-dd"; return formatter.string(from: date)
}

private struct RepositoryRow: View {
    let repo: Repository
    let selected: Bool
    var body: some View {
        GeometryReader { geometry in
            let widths = columnWidths(geometry.size.width)
            HStack(spacing: 0) {
                HStack(spacing: 10) {
                    WatchIcon(name: "book-open", size: 20, color: WatchStyle.green).frame(width: 40, height: 40).background(WatchStyle.selected).cornerRadius(7)
                    VStack(alignment: .leading, spacing: 4) { Text(repo.repo).font(watchFont(13, bold: true)).lineLimit(1); Text(repo.snapshot?.description.isEmpty == false ? repo.snapshot!.description : "暂无项目描述").font(watchFont(11)).foregroundColor(WatchStyle.muted).lineLimit(2) }
                }.frame(width: widths[0], alignment: .leading)
                HStack(spacing: 4) { WatchIcon(name: repo.platform, size: 15, family: "simple-icons"); Text(["github":"GitHub", "gitea":"Gitea", "codeberg":"Codeberg"][repo.platform] ?? repo.platform).lineLimit(1) }.frame(width: widths[1], alignment: .leading)
                VStack(alignment: .leading, spacing: 3) { ForEach(Array((repo.tags ?? []).prefix(2)), id: \.self) { TagPill(text: $0) } }.frame(width: widths[2], alignment: .leading)
                if widths[3] > 0 {
                    HStack(spacing: 4) { WatchIcon(name: "star", size: 14); Text(compactCount(repo.snapshot?.stars)) }.frame(width: widths[3], alignment: .leading)
                    HStack(spacing: 4) { WatchIcon(name: "git-fork", size: 14); Text(compactCount(repo.snapshot?.forks)) }.frame(width: widths[4], alignment: .leading)
                    Text(relativeDate(repo.snapshot?.pushedAt ?? "")).foregroundColor(WatchStyle.muted).frame(width: widths[5], alignment: .leading)
                }
            }.font(watchFont(11)).padding(.leading, 8).frame(height: 105).frame(maxWidth: .infinity, alignment: .leading)
                .background(selected ? WatchStyle.selected : WatchStyle.surface).cornerRadius(6)
                .overlay(alignment: .leading) { if selected { RoundedRectangle(cornerRadius: 2).fill(WatchStyle.green).frame(width: 4).padding(.vertical, 6) } }
                .overlay(alignment: .bottom) { if !selected { Rectangle().fill(WatchStyle.border).frame(height: 1).padding(.horizontal, 8) } }
        }.frame(height: 105).accessibilityElement(children: .combine)
    }
}

private struct RepositoryInspector: View {
    let repo: Repository
    let open: (String?) -> Void
    let terminal: () -> Void
    let edit: () -> Void
    let remove: () -> Void
    private var releasesURL: String? {
        guard let raw = repo.snapshot?.url, var components = URLComponents(string: raw), ["http", "https"].contains(components.scheme?.lowercased() ?? ""), components.host != nil else { return nil }
        components.path = components.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        components.path = "/" + components.path + "/releases"; components.query = nil; components.fragment = nil
        return components.string
    }
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                HStack(spacing: 10) {
                    WatchIcon(name: "book-open", size: 25, color: WatchStyle.green).frame(width: 54, height: 54).background(WatchStyle.selected).cornerRadius(8)
                    VStack(alignment: .leading, spacing: 4) { Text(repo.repo).font(watchFont(17, bold: true)).lineLimit(1); Text("\(repo.owner) / \(repo.platform.capitalized)").font(watchFont(11)).foregroundColor(WatchStyle.muted).lineLimit(1) }
                    Spacer(); Button(action: { open(repo.snapshot?.url) }) { WatchIcon(name: "external-link", color: WatchStyle.muted) }.buttonStyle(.plain).help("打开仓库链接")
                }
                Text(repo.snapshot?.description.isEmpty == false ? repo.snapshot!.description : "暂无项目描述").font(watchFont(12)).foregroundColor(WatchStyle.muted).frame(maxWidth: .infinity, minHeight: 56, alignment: .topLeading)
                if !(repo.tags ?? []).isEmpty { HStack { ForEach(Array((repo.tags ?? []).prefix(3)), id: \.self) { TagPill(text: $0) } } } else { Text("暂无标签").font(watchFont(11)).foregroundColor(WatchStyle.muted) }
                HStack { metric("star", repo.snapshot?.stars); Spacer(); metric("git-fork", repo.snapshot?.forks); Spacer(); metric("eye", repo.snapshot?.watchers) }.padding(.vertical, 8)
                Divider()
                Group {
                    fact("主要语言", repo.snapshot?.language); fact("许可证", repo.snapshot?.license); fact("开放 Issue", repo.snapshot.map { String($0.issues) }); fact("创建时间", repo.snapshot?.createdAt.map(relativeDate)); fact("最近更新", repo.snapshot.map { relativeDate($0.pushedAt) })
                    if !repo.error.isEmpty { Text(repo.error).font(watchFont(11)).foregroundColor(.red) } else if !repo.change.isEmpty { Text(repo.change).font(watchFont(11)).foregroundColor(WatchStyle.muted) }
                    Divider().padding(.top, 4)
                }
                HStack { Text("最新发行版").font(watchFont(12, bold: true)); Spacer(); Button("查看全部") { open(releasesURL) }.buttonStyle(.plain).font(watchFont(11)).foregroundColor(WatchStyle.green) }
                Button(action: { open(repo.snapshot?.releaseUrl) }) {
                    VStack(alignment: .leading, spacing: 5) { Text(repo.snapshot?.release.isEmpty == false ? repo.snapshot!.release : "暂无发行版").font(watchFont(12, bold: true)); Text(repo.snapshot?.releaseNotes?.isEmpty == false ? repo.snapshot!.releaseNotes! : "暂无可用的发行版信息").font(watchFont(11)).foregroundColor(WatchStyle.muted).lineLimit(2) }.frame(maxWidth: .infinity, minHeight: 68, alignment: .leading).padding(.horizontal, 10).background(WatchStyle.header).cornerRadius(5)
                }.buttonStyle(.plain).disabled(repo.snapshot?.releaseUrl.isEmpty != false)
                Group {
                    action("打开仓库", icon: "external-link") { open(repo.snapshot?.url) }
                    action("查看发行版", icon: "tag") { open(releasesURL) }
                    action("在终端打开", icon: "terminal", perform: terminal)
                    action("编辑仓库", icon: "pencil", perform: edit)
                    Button(action: {
                        showPopupMenu([
                            ("复制仓库地址", false, {
                                if let url = repo.snapshot?.url, !url.isEmpty { NSPasteboard.general.clearContents(); NSPasteboard.general.setString(url, forType: .string) }
                            }),
                            ("移除仓库", false, { remove() }),
                        ])
                    }) { HStack { Spacer(); Text("更多操作"); Spacer() }.frame(maxWidth: .infinity) }.buttonStyle(WatchButtonStyle())
                }
            }.padding(20).frame(maxWidth: .infinity, alignment: .topLeading)
        }
    }
    private func metric(_ icon: String, _ value: Int?) -> some View { HStack(spacing: 5) { WatchIcon(name: icon); Text(compactCount(value)).font(watchFont(12)) } }
    private func fact(_ title: String, _ value: String?) -> some View { HStack { Text(title).foregroundColor(WatchStyle.muted); Spacer(); Text(value?.isEmpty == false ? value! : "—").lineLimit(1) }.font(watchFont(11)) }
    private func action(_ title: String, icon: String, perform: @escaping () -> Void) -> some View {
        Button(action: perform) { HStack { Spacer(); WatchIcon(name: icon, size: 14, color: WatchStyle.green); Text(title); Spacer() }.frame(maxWidth: .infinity) }.buttonStyle(WatchButtonStyle())
    }
}
