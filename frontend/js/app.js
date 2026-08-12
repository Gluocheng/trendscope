/* TrendScope 前端逻辑:时间窗口切换、数据加载、雷达图与榜单渲染。 */
(function () {
  "use strict";

  const tabs = document.querySelectorAll(".tab");
  const radarEl = document.getElementById("radarChart");
  const repoList = document.getElementById("repoList");
  const repoTpl = document.getElementById("repoItemTpl");
  const metaBar = document.getElementById("metaBar");
  const snapshotAt = document.getElementById("snapshotAt");
  const requestedAt = document.getElementById("requestedAt");
  const toast = document.getElementById("toast");

  let radarChart = null;
  let currentWindow = "day";

  // 各语言主题色,用于雷达图与榜单标签
  const LANG_COLORS = {
    Go: "#00ADD8",
    Python: "#3572A5",
    TypeScript: "#3178c6",
    JavaScript: "#f1e05a",
    Rust: "#dea584",
    Java: "#b07219",
    Kotlin: "#A97BFF",
    Swift: "#F05138",
    "C++": "#f34b7d",
    "C#": "#178600",
    Ruby: "#701516",
    PHP: "#4F5D95",
    Default: "#58a6ff",
  };

  async function fetchJSON(url) {
    const resp = await fetch(url);
    const body = await resp.json();
    if (!resp.ok || body.error) {
      throw new Error(body.error || "HTTP " + resp.status);
    }
    return body;
  }

  function showToast(msg) {
    toast.textContent = msg;
    toast.hidden = false;
    clearTimeout(showToast._t);
    showToast._t = setTimeout(() => { toast.hidden = true; }, 5000);
  }

  function langColor(lang) {
    return LANG_COLORS[lang] || LANG_COLORS.Default;
  }

  function formatTime(iso) {
    if (!iso) return "-";
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleString("zh-CN", { hour12: false });
  }

  function renderMeta(meta) {
    if (!meta) return;
    metaBar.hidden = false;
    snapshotAt.textContent = formatTime(meta.snapshot_at);
    requestedAt.textContent = formatTime(meta.requested_at);
  }

  function renderRadar(scores) {
    if (!radarChart) {
      radarChart = echarts.init(radarEl, null, { renderer: "canvas" });
      window.addEventListener("resize", () => radarChart && radarChart.resize());
    }
    if (!scores || scores.length === 0) {
      radarChart.clear();
      radarChart.hideLoading();
      radarChart.setOption({
        title: {
          text: "暂无数据\n请先运行抓取服务",
          left: "center",
          top: "middle",
          textStyle: { color: "#8b98a5", fontSize: 14, fontWeight: "normal" },
        },
      });
      return;
    }
    radarChart.hideLoading();

    const langs = scores.map((s) => s.language);
    const values = scores.map((s) => s.score);
    const maxScore = Math.max(...values, 1);

    const option = {
      tooltip: {
        backgroundColor: "rgba(22,28,34,0.95)",
        borderColor: "#232b33",
        textStyle: { color: "#e6edf3", fontSize: 12 },
        formatter(params) {
          const s = scores[params.dataIndex];
          return [
            `<b>${s.language}</b>`,
            `活跃度: ${s.score.toLocaleString()}`,
            `仓库数: ${s.count}`,
            `平均星标: ${s.avg_stars.toFixed(0)}`,
          ].join("<br>");
        },
      },
      radar: {
        indicator: langs.map((l) => ({
          name: l,
          max: Math.ceil(maxScore * 1.15),
        })),
        radius: "65%",
        splitNumber: 4,
        axisName: { color: "#e6edf3", fontSize: 12 },
        splitLine: { lineStyle: { color: "#2a323b" } },
        splitArea: { areaStyle: { color: ["rgba(255,255,255,0.02)", "rgba(255,255,255,0.05)"] } },
        axisLine: { lineStyle: { color: "#2a323b" } },
      },
      series: [
        {
          type: "radar",
          data: [
            {
              value: values,
              name: "活跃度",
              lineStyle: { color: "#3fb950", width: 2 },
              itemStyle: { color: "#3fb950" },
              areaStyle: { color: "rgba(63,185,80,0.25)" },
              symbolSize: 4,
            },
          ],
        },
      ],
    };
    radarChart.setOption(option, true);
  }

  function renderRepos(repos) {
    repoList.innerHTML = "";
    if (!repos || repos.length === 0) {
      const li = document.createElement("li");
      li.className = "empty-state";
      li.textContent = "暂无数据。请先运行抓取服务填充数据。";
      repoList.appendChild(li);
      return;
    }
    const frag = document.createDocumentFragment();
    repos.forEach((repo, i) => {
      const node = repoTpl.content.cloneNode(true);
      node.querySelector(".repo-rank").textContent = i + 1;
      const link = node.querySelector(".repo-name");
      link.textContent = repo.full_name;
      link.href = repo.html_url;
      node.querySelector(".repo-desc").textContent =
        repo.description || "暂无描述";
      const tag = node.querySelector(".lang-tag");
      tag.textContent = repo.language || "未知";
      tag.style.color = langColor(repo.language);
      tag.style.borderColor = langColor(repo.language);
      node.querySelector(".repo-stars b").textContent =
        repo.stars.toLocaleString();
      frag.appendChild(node);
    });
    repoList.appendChild(frag);
  }

  async function load() {
    try {
      const radarBody = await fetchJSON(
        `/api/radar?window=${currentWindow}`
      );
      const reposBody = await fetchJSON(
        `/api/repos?window=${currentWindow}`
      );
      renderRadar(radarBody.data);
      renderRepos(reposBody.data);
      renderMeta(reposBody.meta || radarBody.meta);
    } catch (err) {
      console.error(err);
      showToast("数据加载失败: " + err.message);
      renderRadar([]);
      renderRepos([]);
    }
  }

  tabs.forEach((tab) => {
    tab.addEventListener("click", () => {
      if (tab.classList.contains("active")) return;
      tabs.forEach((t) => t.classList.remove("active"));
      tab.classList.add("active");
      currentWindow = tab.dataset.window;
      load();
    });
  });

  load();
})();
