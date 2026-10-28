// Page: 上游 Key 池 — Zen / Go 上游 Key 的调度状态、冷却重置和 Proxy 绑定。
import { api } from "../api.js";
import { refreshMonitor } from "../data.js";
import { t, onChange } from "../i18n.js";
import * as store from "../store.js";
import { $, el, emptyRow, localTime, toast } from "../ui.js";

function visibleKeys() {
  const tier = $("#upstream-key-tier")?.value || "zen";
  const keys = store.get("monitor")?.resources?.keys || [];
  return tier === "all" ? keys : keys.filter((key) => key.tier === tier);
}

function render() {
  const resources = store.get("monitor")?.resources;
  const keys = visibleKeys();
  const proxies = resources?.proxies || [];
  const cooling = keys.filter((key) => key.cooldown_until).length;
  const available = keys.length - cooling;
  const usableProxies = proxies.filter((proxy) => proxy.healthy && !proxy.blocked_until).length;

  $("#uk-total").textContent = String(keys.length);
  $("#uk-available").textContent = String(available);
  $("#uk-cooling").textContent = String(cooling);
  $("#uk-proxies").textContent = `${usableProxies}/${proxies.length}`;

  const host = $("#upstream-key-rows");
  host.replaceChildren();
  for (const key of keys) {
    const proxy = proxies.find((item) => item.index === key.proxy_index);
    const proxyText = proxy
      ? `${proxy.address}${proxy.direct ? ` · ${t("直连", "upstreamKeys.direct")}` : ""}${
          proxy.blocked_until ? ` · ${t("封锁中", "state.blocked")}` : ""
        }`
      : "—";
    const reset = el("button", {
      type: "button",
      class: "btn btn-sm btn-ghost",
      text: t("重置冷却", "upstreamKeys.reset"),
      disabled: !key.cooldown_until || !key.fingerprint,
    });
    reset.addEventListener("click", async () => {
      reset.disabled = true;
      try {
        await api(
          `/api/upstream-keys/${encodeURIComponent(key.tier)}/${encodeURIComponent(key.fingerprint)}/reset`,
          { method: "POST", body: "{}" },
        );
        await refreshMonitor();
        toast(t("Key 冷却已重置", "toast.upstreamKey.reset"));
      } catch (error) {
        toast(error.message, true);
        reset.disabled = !key.cooldown_until;
      }
    });
    host.append(
      el("tr", {}, [
        el("td", { class: "mono", text: key.id }),
        el("td", { text: key.tier }),
        el("td", {
          class: `status ${key.cooldown_until ? "warn" : "ok"}`,
          text: key.cooldown_until
            ? `${t("冷却中", "state.cooling")} ${localTime(key.cooldown_until)}`
            : t("可用", "state.available"),
        }),
        el("td", { class: "num", text: String(key.failures || 0) }),
        el("td", { class: "mono", text: proxyText }),
        el("td", { class: "mono", text: key.cooldown_until ? localTime(key.cooldown_until) : "—" }),
        el("td", {}, [reset]),
      ]),
    );
  }
  if (!keys.length) {
    emptyRow(
      host,
      7,
      $("#upstream-key-tier")?.value === "go"
        ? t("尚未配置 Go Key", "upstreamKeys.noGo")
        : t("尚未配置 Zen Key", "upstreamKeys.noZen"),
    );
  }
}

async function refresh() {
  const button = $("#upstream-key-refresh");
  button.disabled = true;
  try {
    await refreshMonitor();
    render();
  } finally {
    button.disabled = false;
  }
}

export default {
  id: "upstream-keys",
  title: "上游 Key 池",
  sub: "Zen / Go 上游 Key 的调度、冷却与 Proxy 绑定。",
  init() {
    $("#upstream-key-tier").addEventListener("change", render);
    $("#upstream-key-refresh").addEventListener("click", refresh);
    store.on("monitor", render);
    onChange(render);
  },
  show: render,
};
