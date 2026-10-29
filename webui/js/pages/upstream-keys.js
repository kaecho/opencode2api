// Page: 上游 Key 池 — Zen / Go 上游 Key 的调度状态、冷却重置和 Proxy 绑定。
import { api } from "../api.js";
import { loadConfig, refreshMonitor } from "../data.js";
import { t, onChange } from "../i18n.js";
import * as store from "../store.js";
import { $, confirmDialog, el, emptyRow, localTime, toast } from "../ui.js";

function fillPolicy() {
  const performance = store.get("config")?.performance;
  if (!performance) return;
  $("#zen-key-short-freeze").value = performance.zen_key_short_freeze_seconds ?? 600;
  $("#zen-key-long-freeze").value = performance.zen_key_long_freeze_seconds ?? 86400;
  $("#zen-key-429-threshold").value = performance.zen_key_429_threshold ?? 3;
  $("#zen-key-429-window").value = performance.zen_key_429_window_seconds ?? 86400;
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
  const cooling = keys.filter((key) => key.cooldown_until || key.long_freeze_until || key.quota_exhausted).length;
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
          class: `status ${key.quota_exhausted || key.long_freeze_until || key.cooldown_until ? "warn" : "ok"}`,
          text: key.quota_exhausted || key.long_freeze_until
            ? `${t("额度用完", "upstreamKeys.quotaExhausted")} ${localTime(key.long_freeze_until)}`
            : key.cooldown_until
              ? `${t("冻结中", "upstreamKeys.shortFrozen")} ${localTime(key.cooldown_until)}`
              : t("可用", "state.available"),
        }),
        el("td", { class: "num", text: String(key.failures || 0) }),
        el("td", { class: "mono", text: proxyText }),
        el("td", { class: "mono", text: key.long_freeze_until ? localTime(key.long_freeze_until) : key.cooldown_until ? localTime(key.cooldown_until) : "—" }),
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

async function savePolicy(event) {
  event.preventDefault();
  const button = $("#zen-key-policy-save");
  button.disabled = true;
  try {
    await api("/api/upstream-keys/policy", {
      method: "PUT",
      body: JSON.stringify({
        short_freeze_seconds: Number($("#zen-key-short-freeze").value),
        long_freeze_seconds: Number($("#zen-key-long-freeze").value),
        threshold: Number($("#zen-key-429-threshold").value),
        window_seconds: Number($("#zen-key-429-window").value),
      }),
    });
    await Promise.all([loadConfig(), refreshMonitor()]);
    toast(t("调度规则已保存", "toast.upstreamKey.policySaved"));
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
    $("#zen-key-policy-form").addEventListener("submit", savePolicy);
    store.on("monitor", render);
    store.on("config", fillPolicy);
    onChange(render);
  },
  show() {
    fillPolicy();
    render();
  },
};
