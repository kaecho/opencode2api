// Page: 配置中心 — masked config editing with validation and hot reload.
import { api } from "../api.js";
import { loadConfig, loadDebugModels } from "../data.js";
import * as store from "../store.js";
import { t, onChange } from "../i18n.js";
import { $, $$, bindTabs, el, toast } from "../ui.js";

function renderChips(name) {
  const host = $(`#${name}-chips`);
  host.replaceChildren();
  (store.get("config")?.[name] || []).forEach((item) => {
    const remove = el("button", { type: "button", title: t("移除", "common.remove"), text: "×" });
    const chip = el("span", { class: "chip", dataset: { id: item.id } });
    remove.addEventListener("click", () => chip.remove());
    chip.append(el("span", { text: item.display }), remove);
    host.append(chip);
  });
}

function renderRestartNotice(value) {
  const notice = $("#restart-notice");
  if (value?.restart_required_fields?.length) {
    notice.textContent = t(
      `已保存但尚未生效：${value.restart_required_fields.join(", ")}。当前 API ${value.effective.listen}、WebUI ${value.effective.webui_listen}。请重启进程。`,
      "config.restartNotice",
      value.restart_required_fields.join(", "),
      value.effective.listen,
      value.effective.webui_listen,
    );
    notice.classList.remove("hidden");
  } else {
    notice.classList.add("hidden");
  }
}

function secretInputs(name) {
  const kept = [...$(`#${name}-chips`).children].map((node) => ({ id: node.dataset.id }));
  const added = $(`#${name}-new`)
    .value.split(/\r?\n/)
    .map((value) => value.trim())
    .filter(Boolean)
    .map((value) => ({ value }));
  return kept.concat(added);
}

const numberValue = (selector) => Number($(selector).value);

function fillConfig(value) {
  if (!value) return;
  $("#c-listen").value = value.listen;
  $("#c-prefer").value = value.prefer;
  $("#c-web-listen").value = value.webui.listen;
  $("#c-session").value = value.webui.session_ttl_minutes;
  $("#c-web-enabled").checked = value.webui.enabled;
  $("#c-anonymous").checked = !!value.anonymous;
  const warp = value.warp || {};
  $("#c-warp-enabled").checked = !!warp.enabled;
  $("#c-warp-pool").value = warp.pool_size || 2;
  $("#c-warp-stack").value = warp.stack || "ipv6";
  $("#c-warp-source").value = warp.source_ipv6 || "";
  $("#c-proxyfile").value = value.proxyfile;
  $("#c-proxy-block-threshold").value = value.performance.proxy_block_threshold ?? 3;
  $("#c-proxy-block-seconds").value = value.performance.proxy_block_seconds ?? 900;
  $("#c-require-proxy").checked = !!value.performance.require_proxy;
  $("#c-up-zen").value = value.upstream.zen;
  $("#c-up-go").value = value.upstream.go;
  $("#c-attempts").value = value.retry.max_attempts;
  $("#c-timeout").value = value.retry.timeout_seconds;
  $("#c-refresh").value = value.models.refresh_seconds;
  $("#c-protocols").value = JSON.stringify(value.models.protocols || {}, null, 2);
  $("#c-free-only").checked = !!value.models.free_only;
  const reasoning = value.reasoning || {};
  $("#c-reasoning-effort").value = reasoning.effort || "";
  $("#c-reasoning-by-model").value = JSON.stringify(reasoning.effort_by_model || {}, null, 2);
  $("#c-idle").value = value.performance.max_idle_conns;
  $("#c-idle-host").value = value.performance.max_idle_conns_per_host;
  $("#c-max-host").value = value.performance.max_conns_per_host;
  $("#c-idle-timeout").value = value.performance.idle_conn_timeout_seconds;
  $("#c-connect").value = value.performance.connect_timeout_seconds;
  $("#c-cooldown").value = value.performance.failure_cooldown_seconds;
  $("#c-attempt-timeout").value = value.performance.attempt_timeout_seconds;
  $("#c-first-event-timeout").value = value.performance.first_event_timeout_seconds || 0;
  $("#c-max-client-tools").value = value.performance.max_client_tools || 0;
  $("#c-zero-width-keywords").value = (value.performance.zero_width_keywords || []).join("\n");
  $("#c-level").value = value.logging.level;
  $("#c-ring").value = value.logging.ring_size;
  $("#c-dump-bodies").checked = !!value.logging.dump_request_bodies;
  renderChips("proxies");
  renderRestartNotice(value);
}

async function save(event) {
  event.preventDefault();
  const current = store.get("config");
  if (!current) return;
  try {
    const update = {
      listen: $("#c-listen").value,
      server_keys: (current.server_keys || []).map((item) => ({ id: item.id })),
      zen_keys: (current.zen_keys || []).map((item) => ({ id: item.id })),
      go_keys: (current.go_keys || []).map((item) => ({ id: item.id })),
      anonymous: $("#c-anonymous").checked,
      warp: {
        enabled: $("#c-warp-enabled").checked,
        pool_size: numberValue("#c-warp-pool"),
        stack: $("#c-warp-stack").value,
        source_ipv6: $("#c-warp-source").value.trim(),
      },
      proxies: secretInputs("proxies"),
      proxyfile: $("#c-proxyfile").value,
      prefer: $("#c-prefer").value,
      upstream: { zen: $("#c-up-zen").value, go: $("#c-up-go").value },
      retry: { max_attempts: numberValue("#c-attempts"), timeout_seconds: numberValue("#c-timeout") },
      models: {
        refresh_seconds: numberValue("#c-refresh"),
        protocols: JSON.parse($("#c-protocols").value || "{}"),
        free_only: $("#c-free-only").checked,
      },
      reasoning: {
        effort: $("#c-reasoning-effort").value,
        effort_by_model: JSON.parse($("#c-reasoning-by-model").value || "{}"),
      },
      performance: {
        ...current.performance,
        max_idle_conns: numberValue("#c-idle"),
        max_idle_conns_per_host: numberValue("#c-idle-host"),
        max_conns_per_host: numberValue("#c-max-host"),
        idle_conn_timeout_seconds: numberValue("#c-idle-timeout"),
        connect_timeout_seconds: numberValue("#c-connect"),
        failure_cooldown_seconds: numberValue("#c-cooldown"),
        attempt_timeout_seconds: numberValue("#c-attempt-timeout"),
        first_event_timeout_seconds: numberValue("#c-first-event-timeout"),
        max_client_tools: numberValue("#c-max-client-tools"),
        reuse_official_tools: $("#c-reuse-official-tools").checked,
        zero_width_keywords: $("#c-zero-width-keywords")
          .value.split(/\r?\n/)
          .map((item) => item.trim())
          .filter(Boolean),
        proxy_block_threshold: numberValue("#c-proxy-block-threshold"),
        proxy_block_seconds: numberValue("#c-proxy-block-seconds"),
        require_proxy: $("#c-require-proxy").checked,
      },
      logging: {
        level: $("#c-level").value,
        ring_size: numberValue("#c-ring"),
        dump_request_bodies: $("#c-dump-bodies").checked,
      },
      webui: {
        enabled: $("#c-web-enabled").checked,
        listen: $("#c-web-listen").value,
        username: current.webui.username,
        session_ttl_minutes: numberValue("#c-session"),
      },
    };
    const result = await api("/api/config", { method: "PUT", body: JSON.stringify(update) });
    store.set("config", result.config);
    $("#proxies-new").value = "";
    await loadDebugModels();
    toast(
      result.result.restart_required
        ? t(
            `已应用；${result.result.restart_fields.join(", ")} 需重启`,
            "toast.config.restart",
            result.result.restart_fields.join(", "),
          )
        : t("配置已保存并立即应用", "toast.config.saved"),
    );
  } catch (error) {
    toast(error.message, true);
  }
}

async function reload() {
  try {
    const result = await api("/api/config/reload", { method: "POST", body: "{}" });
    await loadConfig();
    await loadDebugModels();
    toast(
      result.restart_required
        ? t("磁盘配置已重载，监听字段需重启", "toast.config.reloadedRestart")
        : t("磁盘配置已重载", "toast.config.reloaded"),
    );
  } catch (error) {
    toast(error.message, true);
  }
}



export default {
  id: "config",
  title: "配置中心",
  sub: "验证后原子保存，并热应用可变字段。",
  init() {
    store.on("config", fillConfig);
    bindTabs($("#page-config"));
    $("#config-form").addEventListener("submit", save);
    $("#reload-config").addEventListener("click", reload);
    onChange(() => {
      const value = store.get("config");
      if (value) renderRestartNotice(value);
      // Update chip tooltips in place so pending unsaved chip removals survive a language switch.
      $$(".chips .chip button").forEach((button) => (button.title = t("移除", "common.remove")));
    });
  },
};
