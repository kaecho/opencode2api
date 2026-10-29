// Page: 上游 Key 池 — Zen / Go 上游 Key 的调度状态、冷却重置和 Proxy 绑定。
import { api } from "../api.js";
import { loadConfig, refreshMonitor } from "../data.js";
import { t, onChange } from "../i18n.js";
import * as store from "../store.js";
import { $, confirmDialog, el, emptyRow, remainingDuration, toast } from "../ui.js";

let ticker = null;

function startTicker() {
  if (ticker) return;
  ticker = setInterval(() => {
    for (const cell of document.querySelectorAll("#upstream-key-rows [data-remaining]")) {
      cell.textContent = remainingDuration(cell.dataset.remaining);
    }
  }, 1000);
}

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
    const remove = el("button", {
      type: "button",
      class: "btn btn-sm btn-ghost",
      text: t("删除", "upstreamKeys.delete"),
    });
    remove.addEventListener("click", async () => {
      if (
        !(await confirmDialog({
          title: t("删除上游 Key", "upstreamKeys.deleteTitle"),
          message: t(`将从 ${key.tier} 号池移除尾码 ${key.id}，确定吗？`, "upstreamKeys.deleteConfirm", key.tier, key.id),
          confirmLabel: t("删除", "upstreamKeys.delete"),
          danger: true,
        }))
      ) {
        return;
      }
      remove.disabled = true;
      try {
        await api(`/api/upstream-keys/${encodeURIComponent(key.tier)}/${encodeURIComponent(key.fingerprint)}`, {
          method: "DELETE",
        });
        await Promise.all([refreshMonitor(), loadConfig()]);
        toast(t("Key 已删除", "toast.upstreamKey.deleted"));
      } catch (error) {
        toast(error.message, true);
        remove.disabled = false;
      }
    });
    host.append(
      el("tr", {}, [
        el("td", { class: "mono", text: key.id }),
        el("td", { text: key.tier }),
        el("td", {
          class: `status ${key.cooldown_until ? "warn" : "ok"}`,
          text: key.cooldown_until ? t("冻结中", "upstreamKeys.shortFrozen") : t("可用", "state.available"),
        }),
        el("td", { class: "num", text: String(key.failures || 0) }),
        el("td", { class: "mono", text: proxyText }),
        el("td", {
          class: "mono",
          text: remainingDuration(key.cooldown_until),
          dataset: key.cooldown_until ? { remaining: key.cooldown_until } : {},
        }),
        el("td", {}, [el("div", { class: "key-actions" }, [reset, remove])]),
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

async function add(event) {
  event.preventDefault();
  const tier = $("#upstream-key-add-tier").value;
  const input = $("#upstream-key-new");
  const key = input.value.trim();
  if (!key) {
    toast(t("请填写上游 Key", "upstreamKeys.add.empty"), true);
    return;
  }
  const button = $("#upstream-key-add");
  button.disabled = true;
  try {
    await api(`/api/upstream-keys/${encodeURIComponent(tier)}`, {
      method: "POST",
      body: JSON.stringify({ key }),
    });
    input.value = "";
    await Promise.all([refreshMonitor(), loadConfig()]);
    toast(t("上游 Key 已添加", "toast.upstreamKey.added"));
  } catch (error) {
    toast(error.message, true);
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
    $("#upstream-key-form").addEventListener("submit", add);
    startTicker();
    store.on("monitor", render);
    onChange(render);
  },
  show() {
    render();
  },
};
