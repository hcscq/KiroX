// ===== 概览页面逻辑 =====

var overviewTimer = null;
var taskStatusTimer = null;

function _ovT(key, fallback) {
  if (window.I18N && typeof window.I18N.t === 'function') {
    var v = window.I18N.t(key);
    if (v && v !== key) return v;
  }
  return fallback;
}

// loadOverview 加载概览数据（含账号池统计，3秒刷新）
async function loadOverview() {
  if (!window.go || !window.go.main || !window.go.main.App) return;
  try {
    var data = await window.go.main.App.GetOverview();
    updateOverviewUI(data);
    // 加载 MoeMail 配置统计
    if (typeof loadMoeMailConfigs === 'function') {
      loadMoeMailConfigs();
    }
    // 同步 Google 账号池与有效付款链接统计
    if (typeof window.go.main.App.GetGoogleAccounts === 'function') {
      var googleAccs = await window.go.main.App.GetGoogleAccounts();
      if (googleAccs) {
        var gTotal = googleAccs.length;
        var gAuth = 0;
        var gReady = 0;
        var gUninit = 0;
        var gLinks = 0;
        var now = Date.now();
        for (var i = 0; i < googleAccs.length; i++) {
          var a = googleAccs[i];
          if (a.status === 'authorized') {
            gAuth++;
          } else if (a.status === 'logged_in') {
            gReady++;
          } else {
            gUninit++;
          }

          if (a.paymentUrl) {
            if (!a.paymentExpiresAt || new Date(a.paymentExpiresAt.replace(/-/g, '/')).getTime() > now) {
              gLinks++;
            }
          }
        }
        setText('ov-google-count', gTotal);
        setText('ov-google-auth-count', gAuth);
        setText('ov-payment-links-count', gLinks);

        // 刷新右列健康度卡片与分布条
        setText('ov-sub-total', gTotal);
        setText('ov-sub-auth', gAuth);
        setText('ov-sub-ready', gReady);
        setText('ov-sub-uninit', gUninit);
        setText('ov-sub-links', gLinks);

        var pAuth = gTotal > 0 ? Math.round((gAuth / gTotal) * 100) : 0;
        var pReady = gTotal > 0 ? Math.round((gReady / gTotal) * 100) : 0;
        var pUninit = gTotal > 0 ? Math.max(0, 100 - pAuth - pReady) : 0;

        var barAuth = document.getElementById('ov-bar-auth');
        if (barAuth) barAuth.style.width = pAuth + '%';
        var barReady = document.getElementById('ov-bar-ready');
        if (barReady) barReady.style.width = pReady + '%';
        var barUninit = document.getElementById('ov-bar-uninit');
        if (barUninit) barUninit.style.width = pUninit + '%';
      }
    }
  } catch (e) {
    console.error('加载概览数据失败:', e);
  }
}

// updateOverviewUI 更新概览界面
function updateOverviewUI(data) {
  var kiro = data.kiro || {};

  // 本次任务成功数 + 成功率（来自实时任务状态）
  var taskSuccess = kiro.taskSuccess || 0;
  var taskFailed = kiro.taskFailed || 0;
  var taskTotal = taskSuccess + taskFailed;
  setText('ov-kiro-success', taskSuccess);
  var successRate = taskTotal > 0 ? Math.round(taskSuccess / taskTotal * 100) : 0;
  setText('ov-kiro-success-rate', successRate + '%');
}

// 辅助函数
function setText(id, text) {
  var el = document.getElementById(id);
  if (el) el.textContent = text;
}

// 启动概览定时刷新
function startOverviewTimer() {
  if (overviewTimer) clearInterval(overviewTimer);
  if (taskStatusTimer) {
    clearInterval(taskStatusTimer);
    taskStatusTimer = null;
  }
  loadOverview();
  overviewTimer = setInterval(loadOverview, 3000);
}

// 停止概览定时刷新
function stopOverviewTimer() {
  if (overviewTimer) {
    clearInterval(overviewTimer);
    overviewTimer = null;
  }
  if (taskStatusTimer) {
    clearInterval(taskStatusTimer);
    taskStatusTimer = null;
  }
}
