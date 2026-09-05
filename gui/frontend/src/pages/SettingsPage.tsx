import { useEffect, useState } from "react";
import {
  CheckCircle2,
  Github,
  Monitor,
  Moon,
  Palette,
  Pencil,
  Plus,
  Rocket,
  Save,
  Sun,
  Trash2,
} from "lucide-react";
import {
  checkUpdate,
  getAutostartEnabled,
  getGhProxy,
  getProfiles,
  getStatus,
  getVersion,
  openExternalBrowser,
  saveProfiles,
  setAutostart,
  setGhProxy,
} from "../lib/api";
import { AppProfiles, Profile } from "../lib/types";
import { useThemeContext } from "../lib/ThemeContext";
import type { ThemeMode } from "../lib/theme";
import { Button, Card, Field, Toggle, inputCls } from "../components/ui";
import { usePoll } from "../lib/usePoll";
import { useAsyncAction } from "../lib/useAsyncAction";

const THEME_OPTIONS: { value: ThemeMode; label: string; icon: typeof Sun }[] = [
  { value: "light", label: "浅色", icon: Sun },
  { value: "dark", label: "深色", icon: Moon },
  { value: "system", label: "跟随系统", icon: Monitor },
];

// IPv4/IPv6 策略可选值（对齐 Go XTunnelProfile.IPStrategy 注释）。
const IP_STRATEGY_OPTIONS = ["4", "6", "4,6", "6,4"];

// 新建配置的默认模板（对齐 Go DefaultProfile：不内置任何真实服务器地址/token）。
function blankProfile(n: number): Profile {
  return {
    name: `配置 ${n}`,
    serverURL: "",
    token: "",
    localListen: "socks5://127.0.0.1:11080",
    cidr: "",
    dns: "",
    ech: "",
    blockPorts: "443",
    connections: 3,
    insecure: false,
    fallback: true,
    dialIPs: "",
    ipStrategy: "4",
    dnsCacheTTL: "5m",
  };
}

export default function SettingsPage() {
  const { mode, setMode } = useThemeContext();
  // 运行中锁定编辑（后端运行态不允许改配置）。
  const { data: statusRaw } = usePoll(getStatus, 2000);
  const locked = !!statusRaw?.running;

  const [data, setData] = useState<AppProfiles | null>(null);
  const [editing, setEditing] = useState<Profile | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  // 删除二次确认（点一次进入确认态，5 秒无操作自动取消）。
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  // 开机自启（东哥 09-05 反馈①：从状态页迁入配置页）。
  const [autostart, setAutostartState] = useState(false);
  const [autostartBusy, setAutostartBusy] = useState(false);
  // 关于/检查更新（同反馈①）。
  const [version, setVersion] = useState("…");
  const [updateInfo, setUpdateInfo] = useState<string | null>(null);
  const [updateUrl, setUpdateUrl] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);
  // GitHub 加速地址（反馈④）。
  const [ghProxy, setGhProxyVal] = useState("");
  const [ghProxyDraft, setGhProxyDraft] = useState("");
  // busy 必须解构出来按 key 判断（v0.1.3 bug：runGh 是函数引用恒非 null，
  // loading={runGh !== null} 让保存按钮永久转圈禁用——东哥 09-05 反馈①）。
  const { busy: ghBusy, run: runGh, notice: ghNotice, error: ghError } = useAsyncAction();

  useEffect(() => {
    getAutostartEnabled().then(setAutostartState).catch(() => {});
    getVersion().then(setVersion).catch(() => {});
    getGhProxy()
      .then((v) => {
        setGhProxyVal(v);
        setGhProxyDraft(v);
      })
      .catch(() => {});
  }, []);

  const toggleAutostart = async (v: boolean) => {
    setAutostartBusy(true);
    try {
      await setAutostart(v);
      setAutostartState(v);
    } catch {
      // 失败保持原状，按钮已禁用由 busy 控制
    } finally {
      setAutostartBusy(false);
    }
  };

  const onCheckUpdate = async () => {
    setChecking(true);
    setUpdateInfo(null);
    setUpdateUrl(null);
    try {
      const info = await checkUpdate();
      if (info.has_update) {
        setUpdateInfo(`发现新版本 ${info.tag}（当前 ${version}）`);
        setUpdateUrl(info.url || null);
      } else if (info.latest && info.latest !== "dev") {
        setUpdateInfo(`已是最新版本 ${info.latest}`);
      } else {
        setUpdateInfo("当前为开发版，无法比较版本");
      }
    } catch (e) {
      setUpdateInfo(`检查失败：${String(e)}`);
    } finally {
      setChecking(false);
    }
  };

  const onSaveGhProxy = async () => {
    await runGh("ghproxy", async () => {
      await setGhProxy(ghProxyDraft);
      const saved = await getGhProxy();
      setGhProxyVal(saved);
      setGhProxyDraft(saved);
    });
  };

  const load = async () => {
    try {
      setData(await getProfiles());
      setError(null);
    } catch (e) {
      setError(String(e));
    }
  };

  useEffect(() => {
    void load();
  }, []);

  // 编辑态拷贝，避免直接改 data。
  const setField = <K extends keyof Profile>(key: K, value: Profile[K]) => {
    setEditing((e) => (e ? { ...e, [key]: value } : e));
    setError(null);
  };

  const onAdd = () => {
    const n = (data?.profiles.length ?? 0) + 1;
    setEditing(blankProfile(n));
    setError(null);
    setNotice(null);
  };

  const onEdit = (p: Profile) => {
    setEditing({ ...p });
    setError(null);
    setNotice(null);
  };

  const onSave = async () => {
    if (!editing || !data) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const exists = data.profiles.some((p) => p.name === editing.name);
      const profiles = exists
        ? data.profiles.map((p) => (p.name === editing.name ? editing : p))
        : [...data.profiles, editing];
      // 激活项改名时跟随新名。
      const activeProfile =
        data.activeProfile === editing.name ? editing.name : data.activeProfile;
      await saveProfiles({ activeProfile, profiles });
      setData({ activeProfile, profiles });
      setEditing(null);
      setNotice("配置已保存");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const onActivate = async (name: string) => {
    if (!data) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await saveProfiles({ ...data, activeProfile: name });
      setData({ ...data, activeProfile: name });
      setNotice(`已激活配置：${name}`);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const onDelete = async (name: string) => {
    if (!data) return;
    if (confirmDelete !== name) {
      setConfirmDelete(name);
      setTimeout(() => setConfirmDelete((c) => (c === name ? null : c)), 5000);
      return;
    }
    setConfirmDelete(null);
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const profiles = data.profiles.filter((p) => p.name !== name);
      // 删除激活项时回退到第一个配置（空列表则清空激活名）。
      const activeProfile =
        data.activeProfile === name ? profiles[0]?.name ?? "" : data.activeProfile;
      await saveProfiles({ activeProfile, profiles });
      setData({ activeProfile, profiles });
      setNotice(`已删除配置：${name}`);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      {locked && (
        <div className="rounded-lg bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:bg-amber-950/40 dark:text-amber-300">
          代理运行中，配置已锁定（先停止代理再编辑）。
        </div>
      )}
      {notice && (
        <div className="rounded-lg bg-emerald-50 px-4 py-3 text-sm text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">
          {notice}
        </div>
      )}
      {error && (
        <div className="rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">
          {error}
        </div>
      )}

      <Card
        title="配置管理"
        action={
          <Button onClick={onAdd} disabled={locked}>
            <Plus className="h-4 w-4" /> 新增配置
          </Button>
        }
      >
        {!data ? (
          <p className="text-sm text-slate-500 dark:text-slate-400">加载中…</p>
        ) : data.profiles.length === 0 ? (
          <p className="text-sm text-slate-500 dark:text-slate-400">
            尚无配置。点击右上角「新增配置」创建第一个服务器配置。
          </p>
        ) : (
          <ul className="space-y-3">
            {data.profiles.map((p) => {
              const active = p.name === data.activeProfile;
              return (
                <li
                  key={p.name}
                  className={`rounded-lg border p-4 ${
                    active
                      ? "border-orange-300 bg-orange-50/50 dark:border-orange-700/60 dark:bg-orange-950/20"
                      : "border-slate-200 dark:border-slate-800"
                  }`}
                >
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <p className="truncate text-sm font-semibold text-slate-900 dark:text-slate-100">
                          {p.name}
                        </p>
                        {active && (
                          <span className="flex items-center gap-1 text-xs text-orange-600 dark:text-orange-400">
                            <CheckCircle2 className="h-3.5 w-3.5" /> 已激活
                          </span>
                        )}
                      </div>
                      <p className="mt-1 break-all font-mono text-xs text-slate-500 dark:text-slate-400">
                        {p.serverURL || "（未填写服务器地址）"}
                      </p>
                      <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">
                        {p.localListen} · 连接数 {p.connections} · 策略 {p.ipStrategy}
                        {p.ech ? ` · ECH ${p.ech}` : ""}
                      </p>
                    </div>
                    <div className="flex shrink-0 flex-wrap items-center gap-2">
                      {!active && (
                        <Button variant="secondary" onClick={() => onActivate(p.name)} disabled={locked || busy}>
                          激活
                        </Button>
                      )}
                      <Button variant="secondary" onClick={() => onEdit(p)} disabled={locked}>
                        <Pencil className="h-4 w-4" /> 编辑
                      </Button>
                      <Button
                        variant="danger"
                        onClick={() => onDelete(p.name)}
                        disabled={locked || busy}
                      >
                        <Trash2 className="h-4 w-4" />
                        {confirmDelete === p.name ? "确认删除？" : "删除"}
                      </Button>
                    </div>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </Card>

      {editing && (
        <Card title={`编辑配置：${editing.name}`}>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="名称" hint="配置的唯一标识（激活/切换靠它）">
              <input
                className={inputCls}
                value={editing.name}
                onChange={(e) => setField("name", e.target.value)}
              />
            </Field>
            <Field label="服务器地址" hint="wss://host:port[/path] 或 ws://">
              <input
                className={inputCls}
                value={editing.serverURL}
                onChange={(e) => setField("serverURL", e.target.value)}
                placeholder="wss://cf.example.com:8443"
              />
            </Field>
            <Field label="Token" hint="服务器鉴权令牌（可留空）">
              <input
                type="password"
                className={inputCls}
                value={editing.token}
                onChange={(e) => setField("token", e.target.value)}
              />
            </Field>
            <Field label="本地监听" hint="本机代理监听地址">
              <input
                className={inputCls}
                value={editing.localListen}
                onChange={(e) => setField("localListen", e.target.value)}
                placeholder="socks5://127.0.0.1:11080"
              />
            </Field>
            <Field label="DNS" hint="自定义 DNS（空=默认）">
              <input
                className={inputCls}
                value={editing.dns}
                onChange={(e) => setField("dns", e.target.value)}
              />
            </Field>
            <Field label="ECH" hint="ECH 域名（空=默认 cloudflare-ech.com）">
              <input
                className={inputCls}
                value={editing.ech}
                onChange={(e) => setField("ech", e.target.value)}
              />
            </Field>
            <Field label="UDP 阻断端口" hint="如 443，多个用逗号分隔">
              <input
                className={inputCls}
                value={editing.blockPorts}
                onChange={(e) => setField("blockPorts", e.target.value)}
                placeholder="443"
              />
            </Field>
            <Field label="连接数" hint="每 IP 连接数">
              <input
                type="number"
                min={1}
                className={inputCls}
                value={editing.connections}
                onChange={(e) => setField("connections", Math.max(1, Number(e.target.value) || 1))}
              />
            </Field>
            <Field label="优选 IP" hint="-ip CF 优选 IP/域名，逗号分隔（空=自动）">
              <input
                className={inputCls}
                value={editing.dialIPs}
                onChange={(e) => setField("dialIPs", e.target.value)}
                placeholder="162.159.192.5,162.159.193.10"
              />
            </Field>
            <Field label="IPv4 策略" hint="4=仅 IPv4，6=仅 IPv6，4,6 等=优先顺序">
              <select
                className={inputCls}
                value={editing.ipStrategy}
                onChange={(e) => setField("ipStrategy", e.target.value)}
              >
                {IP_STRATEGY_OPTIONS.map((v) => (
                  <option key={v} value={v}>
                    {v}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="DNS 缓存 TTL" hint="如 5m、1h">
              <input
                className={inputCls}
                value={editing.dnsCacheTTL}
                onChange={(e) => setField("dnsCacheTTL", e.target.value)}
                placeholder="5m"
              />
            </Field>
            <div className="flex flex-col gap-3 sm:col-span-2">
              <div className="flex items-center justify-between gap-4">
                <div>
                  <p className="text-sm font-medium text-slate-800 dark:text-slate-200">
                    Insecure（跳过 TLS 证书校验）
                  </p>
                  <p className="text-xs text-slate-500 dark:text-slate-400">
                    自签名/私有 CA 服务器需开启
                  </p>
                </div>
                <Toggle checked={editing.insecure} onChange={(v) => setField("insecure", v)} label="insecure" />
              </div>
              <div className="flex items-center justify-between gap-4">
                <div>
                  <p className="text-sm font-medium text-slate-800 dark:text-slate-200">
                    Fallback（连接失败时回退直连）
                  </p>
                  <p className="text-xs text-slate-500 dark:text-slate-400">
                    服务器不可达时自动放行直连
                  </p>
                </div>
                <Toggle checked={editing.fallback} onChange={(v) => setField("fallback", v)} label="fallback" />
              </div>
            </div>
          </div>

          <div className="mt-5 flex flex-wrap items-center gap-3">
            <Button onClick={onSave} loading={busy} disabled={locked}>
              <Save className="h-4 w-4" /> 保存配置
            </Button>
            <Button variant="secondary" onClick={() => setEditing(null)} disabled={busy}>
              取消
            </Button>
          </div>
          <p className="mt-3 text-xs text-slate-500 dark:text-slate-400">
            配置保存到 profiles.json；激活配置需重启代理后生效。
          </p>
        </Card>
      )}

      <Card title="GitHub 加速">
        <div className="flex items-start gap-3">
          <Github className="mt-0.5 h-5 w-5 text-orange-500" />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium">加速下载前缀</p>
            <p className="text-xs text-slate-500 dark:text-slate-400">
              GEO 数据库与检查更新经此前缀下载 GitHub 资源；填 <code>off</code> 直连。
              默认 <code>https://gh-proxy.org</code>
            </p>
          </div>
        </div>
        <div className="mt-4 flex flex-wrap items-center gap-3">
          <input
            className={`${inputCls} max-w-md flex-1 font-mono`}
            value={ghProxyDraft}
            onChange={(e) => setGhProxyDraft(e.target.value)}
            placeholder="https://gh-proxy.org 或 off"
            disabled={busy}
          />
          <Button onClick={onSaveGhProxy} loading={ghBusy === "ghproxy"} disabled={busy}>
            <Save className="h-4 w-4" /> 保存
          </Button>
          {ghNotice && <span className="text-sm text-emerald-600 dark:text-emerald-400">{ghNotice}</span>}
          {ghError && <span className="text-sm text-red-600 dark:text-red-400">{ghError}</span>}
        </div>
        <p className="mt-2 text-xs text-slate-500 dark:text-slate-400">
          当前生效：{ghProxy || "https://gh-proxy.org（默认）"}
        </p>
      </Card>

      <Card title="开机自启">
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-start gap-3">
            <Rocket className="mt-0.5 h-5 w-5 text-orange-500" />
            <div>
              <p className="text-sm font-medium">登录后自动启动</p>
              <p className="text-xs text-slate-500 dark:text-slate-400">
                系统登录时自动运行（Windows 注册表 / macOS LaunchAgent / Linux autostart）
              </p>
            </div>
          </div>
          <Toggle checked={autostart} onChange={toggleAutostart} disabled={autostartBusy} />
        </div>
      </Card>

      <Card title="关于">
        <div className="flex items-center gap-3">
          <Rocket className="h-5 w-5 text-orange-500" />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium">x-tunnel-windows {version}</p>
            <p className="text-xs text-slate-500 dark:text-slate-400">
              x-tunnel 客户端（WS/WSS 隧道）
            </p>
          </div>
          <Button variant="secondary" onClick={onCheckUpdate} disabled={checking}>
            {checking ? "检查中…" : "检查更新"}
          </Button>
        </div>
        {updateInfo && (
          <p className="mt-3 text-sm">
            <span className={updateUrl ? "text-amber-600 dark:text-amber-400" : "text-emerald-600 dark:text-emerald-400"}>
              {updateInfo}
            </span>
            {updateUrl && (
              <button
                type="button"
                onClick={() => openExternalBrowser(updateUrl)}
                className="ml-2 text-orange-600 underline dark:text-orange-400"
              >
                前往下载
              </button>
            )}
          </p>
        )}
      </Card>

      <Card title="外观">
        <div className="flex items-start gap-3">
          <Palette className="mt-0.5 h-5 w-5 text-orange-500" />
          <div>
            <p className="text-sm font-medium">主题模式</p>
            <p className="text-xs text-slate-500 dark:text-slate-400">
              跟随系统时自动同步操作系统的明暗主题
            </p>
          </div>
        </div>
        <div className="mt-4 grid grid-cols-3 gap-1 rounded-lg border border-slate-200 bg-slate-50 p-1 dark:border-slate-700 dark:bg-slate-800">
          {THEME_OPTIONS.map(({ value, label, icon: Icon }) => (
            <button
              key={value}
              type="button"
              onClick={() => setMode(value)}
              aria-pressed={mode === value}
              className={`flex items-center justify-center gap-1.5 rounded-md px-2 py-2 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-orange-500/50 ${
                mode === value
                  ? "bg-white text-orange-600 shadow-sm dark:bg-slate-700 dark:text-orange-400"
                  : "text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200"
              }`}
            >
              <Icon className="h-4 w-4 shrink-0" />
              {label}
            </button>
          ))}
        </div>
      </Card>
    </div>
  );
}
