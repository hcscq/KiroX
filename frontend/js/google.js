// ===== Google 账号池与独立 Profile 管理 =====

var _googleAccounts = [];

async function loadGoogleAccounts() {
  try {
    if (!window.go || !window.go.main || !window.go.main.App) return;
    var list = await window.go.main.App.GetGoogleAccounts();
    _googleAccounts = list || [];
    renderGoogleAccounts(_googleAccounts);
  } catch (e) {
    console.error('加载 Google 账号失败:', e);
  }
}

function _escapeGoogleHtml(s) {
  if (typeof _escapeLogHtml === 'function') return _escapeLogHtml(s);
  return String(s || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

var _selectedGoogleEmails = new Set();
var _activeGoogleTasks = new Map(); // lowercase email -> task description

function isGoogleAccountBusy(email) {
  if (!email) return false;
  return _activeGoogleTasks.has(email.toLowerCase().trim());
}

function getGoogleAccountBusyTask(email) {
  if (!email) return '';
  return _activeGoogleTasks.get(email.toLowerCase().trim()) || '';
}

function setGoogleAccountBusy(email, taskName) {
  if (!email) return;
  var norm = email.toLowerCase().trim();
  _activeGoogleTasks.set(norm, taskName || '执行中');
  updateAccountRowBusyState(norm);
}

function clearGoogleAccountBusy(email) {
  if (!email) return;
  var norm = email.toLowerCase().trim();
  _activeGoogleTasks.delete(norm);
  updateAccountRowBusyState(norm);
}

function updateAccountRowBusyState(normEmail) {
  var rows = document.querySelectorAll('#google-account-list tr');
  rows.forEach(function(row) {
    var check = row.querySelector('.google-row-check');
    if (check && check.value && check.value.toLowerCase().trim() === normEmail) {
      var isBusy = _activeGoogleTasks.has(normEmail);
      var btns = row.querySelectorAll('button');
      btns.forEach(function(btn) {
        btn.disabled = isBusy;
        if (isBusy) {
          btn.style.opacity = '0.4';
          btn.style.pointerEvents = 'none';
        } else {
          btn.style.opacity = '';
          btn.style.pointerEvents = '';
        }
      });
      var pill = row.querySelector('.status-pill');
      if (pill) {
        if (isBusy) {
          if (!pill.hasAttribute('data-orig-html')) {
            pill.setAttribute('data-orig-html', pill.innerHTML);
            pill.setAttribute('data-orig-class', pill.className);
          }
          pill.className = 'status-pill warning';
          pill.innerHTML = '<span class="status-pill-dot" style="background:#eab308;"></span>' + _escapeGoogleHtml(_activeGoogleTasks.get(normEmail) || '执行中...');
        } else if (pill.hasAttribute('data-orig-html')) {
          pill.className = pill.getAttribute('data-orig-class') || 'status-pill default';
          pill.innerHTML = pill.getAttribute('data-orig-html') || '';
          pill.removeAttribute('data-orig-html');
          pill.removeAttribute('data-orig-class');
        }
      }
    }
  });
}

function toggleSelectAllGoogle(masterCheckbox) {
  var checked = !!(masterCheckbox && masterCheckbox.checked);
  _selectedGoogleEmails.clear();
  var cbs = document.querySelectorAll('.google-row-check');
  for (var i = 0; i < cbs.length; i++) {
    cbs[i].checked = checked;
    if (checked && cbs[i].value) {
      _selectedGoogleEmails.add(cbs[i].value);
    }
  }
}

function updateGoogleSelection() {
  _selectedGoogleEmails.clear();
  var cbs = document.querySelectorAll('.google-row-check');
  var allChecked = cbs.length > 0;
  for (var i = 0; i < cbs.length; i++) {
    if (cbs[i].checked) {
      _selectedGoogleEmails.add(cbs[i].value);
    } else {
      allChecked = false;
    }
  }
  var master = document.getElementById('google-select-all');
  if (master) master.checked = allChecked && cbs.length > 0;
}

function getSelectedGoogleEmails() {
  return Array.from(_selectedGoogleEmails);
}

function renderGoogleAccounts(accounts) {
  var total = accounts ? accounts.length : 0;
  var authorized = 0;
  var ready = 0;
  var uninit = 0;
  var validLinks = 0;
  var now = Date.now();

  if (accounts) {
    for (var j = 0; j < accounts.length; j++) {
      var item = accounts[j];
      if (item.status === 'authorized') {
        authorized++;
      } else if (item.status === 'logged_in') {
        ready++;
      } else {
        uninit++;
      }
      if (item.paymentUrl) {
        if (!item.paymentExpiresAt || new Date(item.paymentExpiresAt.replace(/-/g, '/')).getTime() > now) {
          validLinks++;
        }
      }
    }
  }

  var countEl = document.getElementById('google-count');
  if (countEl) countEl.innerText = total;
  var badgeEl = document.getElementById('tab-badge-google');
  if (badgeEl) badgeEl.innerText = total;

  var elTotal = document.getElementById('google-stat-total');
  if (elTotal) elTotal.innerText = total;
  var elAuth = document.getElementById('google-stat-authorized');
  if (elAuth) elAuth.innerText = authorized;
  var elReady = document.getElementById('google-stat-ready');
  if (elReady) elReady.innerText = ready;
  var elUninit = document.getElementById('google-stat-uninit');
  if (elUninit) elUninit.innerText = uninit;
  var elLinks = document.getElementById('google-stat-links');
  if (elLinks) elLinks.innerText = validLinks;

  var tbody = document.getElementById('google-account-list');
  if (!tbody) return;

  if (!accounts || accounts.length === 0) {
    tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:var(--text-muted);padding:40px 20px;" data-i18n="accounts.noGoogle">暂无 Google 账号，请点击上方“添加账号”导入</td></tr>';
    if (window.I18N && window.I18N.apply) window.I18N.apply();
    return;
  }

  var html = '';
  for (var i = 0; i < accounts.length; i++) {
    var acc = accounts[i];
    var isReady = acc.status === 'logged_in' || acc.status === 'authorized';
    var isAuthorized = acc.status === 'authorized';
    var statusPillClass = 'status-pill default';
    if (isAuthorized) {
      statusPillClass = 'status-pill success';
    } else if (isReady) {
      statusPillClass = 'status-pill info';
    } else if (acc.status === 'error') {
      statusPillClass = 'status-pill danger';
    }
    var statusText = '未登录';
    var statusTooltip = acc.statusMsg || '';
    if (isAuthorized) {
      statusText = '已授权';
      if (!statusTooltip) statusTooltip = '已完成 Kiro 官方授权';
    } else if (isReady) {
      statusText = '已就绪';
      if (!statusTooltip) statusTooltip = '本地 Chrome 会话有效';
    } else if (acc.status === 'logging_in') {
      statusText = '登录中';
    } else if (acc.status === 'error') {
      statusText = '异常';
    }

    var normEmail = (acc.email || '').toLowerCase().trim();
    var isBusy = _activeGoogleTasks.has(normEmail);
    var busyDesc = isBusy ? _activeGoogleTasks.get(normEmail) : '';
    if (isBusy) {
      statusPillClass = 'status-pill warning';
      statusText = busyDesc || '执行中...';
      statusTooltip = '后台正在执行自动化任务，请稍候';
    }

    var busyBtnAttr = isBusy ? ' disabled style="padding:0 6px;opacity:0.4;pointer-events:none;"' : ' style="padding:0 6px;"';

    var proxyText = acc.proxy
      ? '<div style="font-size:11px;font-family:var(--font-mono);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:130px;" title="' + _escapeGoogleHtml(acc.proxy) + '">' + _escapeGoogleHtml(acc.proxy) + '</div>'
      : '<span style="font-size:11px;color:var(--text-muted);">直连</span>';
    var has2FA = !!acc.totpSecret;
    var isChecked = _selectedGoogleEmails.has(acc.email);

    // 付款链接列渲染 (整齐双行结构，与已生成状态高度完全一致)
    var paymentHtml = '';
    if (acc.paymentUrl) {
      var planTitle = acc.planType || 'KIRO_PRO';
      var planBadge = 'Pro ($20)';
      if (planTitle === 'KIRO_PRO_PLUS') planBadge = 'Pro+ ($40)';
      else if (planTitle === 'KIRO_PRO_MAX') planBadge = 'Pro Max ($100)';
      else if (planTitle === 'KIRO_POWER') planBadge = 'Power ($200)';

      var expireDesc = '';
      if (acc.paymentExpiresAt) {
        var diffMs = new Date(acc.paymentExpiresAt.replace(/-/g, '/')).getTime() - Date.now();
        var diffHours = Math.floor(diffMs / (1000 * 3600));
        if (diffMs <= 0) {
          expireDesc = '<span class="badge badge-danger" style="font-size:10px;padding:1px 5px;">已过期</span>';
        } else {
          expireDesc = '<span class="badge badge-success" style="font-size:10px;padding:1px 5px;" title="生成于: ' + _escapeGoogleHtml(acc.paymentCreatedAt || '') + '\n失效于: ' + _escapeGoogleHtml(acc.paymentExpiresAt) + '">剩 ' + diffHours + 'h</span>';
        }
      }

      paymentHtml += '<div style="display:flex;flex-direction:column;gap:4px;">';
      paymentHtml += '  <div style="display:flex;align-items:center;gap:4px;">';
      paymentHtml += '    <span class="badge badge-info" style="font-size:10px;padding:1px 6px;">' + planBadge + '</span>';
      paymentHtml += expireDesc;
      paymentHtml += '  </div>';
      paymentHtml += '  <div style="display:flex;gap:4px;align-items:center;white-space:nowrap;">';
      paymentHtml += '    <button onclick="copyPaymentUrl(\'' + _escapeGoogleHtml(acc.paymentUrl) + '\')" class="btn btn-action-mini"' + (isBusy ? ' disabled style="padding:0 6px;opacity:0.4;pointer-events:none;"' : ' style="padding:0 6px;"') + ' title="复制 24h 免登录 Stripe 结账链接">';
      paymentHtml += '      <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>';
      paymentHtml += '      <span>复制</span>';
      paymentHtml += '    </button>';
      paymentHtml += '    <button onclick="openAutofillStripeModal(\'' + _escapeGoogleHtml(acc.email) + '\', \'' + _escapeGoogleHtml(acc.paymentUrl) + '\')" class="btn btn-action-mini purple"' + (isBusy ? ' disabled style="padding:0 6px;opacity:0.4;pointer-events:none;"' : ' style="padding:0 6px;"') + ' title="打开仿真自动填卡工具">';
      paymentHtml += '      <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2"><rect x="1" y="4" width="22" height="16" rx="2" ry="2"/><line x1="1" y1="10" x2="23" y2="10"/></svg>';
      paymentHtml += '      <span>填卡</span>';
      paymentHtml += '    </button>';
      paymentHtml += '    <button onclick="generateGooglePaymentLink(\'' + _escapeGoogleHtml(acc.email) + '\')" class="btn btn-action-mini"' + (isBusy ? ' disabled style="padding:0 5px;opacity:0.4;pointer-events:none;"' : ' style="padding:0 5px;"') + ' title="重新为此账号生成付款链接">';
      paymentHtml += '      <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2"><path d="M21.5 2v6h-6M21.34 15.57a10 10 0 1 1-.57-8.38l5.67-5.67"/></svg>';
      paymentHtml += '    </button>';
      paymentHtml += '  </div>';
      paymentHtml += '</div>';
    } else {
      paymentHtml += '<div style="display:flex;flex-direction:column;gap:4px;">';
      paymentHtml += '  <div style="display:flex;align-items:center;gap:4px;">';
      paymentHtml += '    <span class="badge badge-secondary" style="font-size:10px;padding:1px 6px;">未生成链接</span>';
      paymentHtml += '  </div>';
      paymentHtml += '  <div style="display:flex;gap:4px;align-items:center;white-space:nowrap;">';
      paymentHtml += '    <button onclick="generateGooglePaymentLink(\'' + _escapeGoogleHtml(acc.email) + '\')" class="btn btn-action-mini purple"' + (isBusy ? ' disabled style="padding:0 6px;opacity:0.4;pointer-events:none;"' : ' style="padding:0 6px;"') + ' title="生成 Stripe 24h 免登录长效付款链接">';
      paymentHtml += '      <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>';
      paymentHtml += '      <span>生成链接</span>';
      paymentHtml += '    </button>';
      paymentHtml += '  </div>';
      paymentHtml += '</div>';
    }

    var activeTime = acc.lastUsedAt || acc.addedAt || '';
    var activeHtml = activeTime ? '<div style="font-size:11px;color:var(--text-muted);margin-top:2px;display:flex;align-items:center;gap:3px;"><span style="opacity:0.7;">活跃:</span> ' + _escapeGoogleHtml(activeTime) + '</div>' : '';

    html += '<tr>';
    html += '<td style="text-align:center;padding:10px 8px;"><input type="checkbox" class="google-row-check" value="' + _escapeGoogleHtml(acc.email) + '" onchange="updateGoogleSelection()" ' + (isChecked ? 'checked' : '') + ' style="cursor:pointer;"></td>';
    html += '<td style="padding:10px 10px;"><div style="font-family:var(--font-mono);font-size:12.5px;font-weight:600;color:var(--text);line-height:1.3;word-break:break-all;">' + _escapeGoogleHtml(acc.email) + '</div>' + activeHtml + '</td>';
    html += '<td style="padding:10px 10px;">' + proxyText + '</td>';
    html += '<td style="padding:10px 10px;"><span class="' + statusPillClass + '" title="' + _escapeGoogleHtml(statusTooltip) + '"><span class="status-pill-dot"' + (isBusy ? ' style="background:#eab308;"' : '') + '></span>' + _escapeGoogleHtml(statusText) + '</span></td>';
    html += '<td style="padding:10px 10px;">' + paymentHtml + '</td>';
    html += '<td style="padding:10px 14px;text-align:right;white-space:nowrap;">';
    html += '  <div style="display:inline-flex;gap:4px;justify-content:flex-end;align-items:center;white-space:nowrap;">';
    html += '    <button onclick="openGoogleManageModal(\'' + _escapeGoogleHtml(acc.email) + '\')" class="btn btn-action-mini manage"' + busyBtnAttr + ' title="打开此 Google 账号的安全管理与 2FA 修改面板">';
    html += '      <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>';
    html += '      <span>管理</span>';
    html += '    </button>';
    var hasKiroCred = !!(acc.kiroRefreshToken || acc.kiroAccessToken);
    var credBtnClass = hasKiroCred ? 'btn btn-action-mini purple' : 'btn btn-action-mini';
    var credBtnTitle = hasKiroCred ? 'Kiro 会话凭据已就绪，点击查看、复制或导出 JSON' : '点击提取并导出该账号的 Kiro 凭据 JSON';
    html += '    <button onclick="openGoogleManageModal(\'' + _escapeGoogleHtml(acc.email) + '\', \'kiro\')" class="' + credBtnClass + '"' + busyBtnAttr + ' title="' + credBtnTitle + '">';
    html += '      <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 2l-2 2m-2-2l2 2"/><path d="M15.5 7.5L20 3m-4.5 4.5a6.5 6.5 0 1 1-9.19 9.19 6.5 6.5 0 0 1 9.19-9.19zM8.5 15.5l.01.01"/></svg>';
    html += '      <span>' + (hasKiroCred ? '🔑凭据' : '凭据') + '</span>';
    html += '    </button>';
    html += '    <button onclick="autoLoginGoogleAccount(\'' + _escapeGoogleHtml(acc.email) + '\')" class="btn btn-action-mini success"' + busyBtnAttr + ' title="全自动仿真打字输入账号、密码、2FA及辅助邮箱完成登录">';
    html += '      <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>';
    html += '      <span>登录</span>';
    html += '    </button>';
    html += '    <button onclick="authorizeKiroWithGoogle(\'' + _escapeGoogleHtml(acc.email) + '\')" class="btn btn-action-mini primary"' + busyBtnAttr + ' title="拉起此环境直接进入 Kiro 官方 Google OAuth 授权">';
    html += '      <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"/><polyline points="10 17 15 12 10 7"/><line x1="15" y1="12" x2="3" y2="12"/></svg>';
    html += '      <span>授权</span>';
    html += '    </button>';
    html += '    <button onclick="openGoogleProfile(\'' + _escapeGoogleHtml(acc.email) + '\')" class="btn btn-action-mini"' + busyBtnAttr + ' title="拉起独立 Chrome 窗口以供手动登录或查看环境">';
    html += '      <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>';
    html += '      <span>环境</span>';
    html += '    </button>';

    if (has2FA) {
      html += '    <button onclick="copyGoogleTOTP(\'' + _escapeGoogleHtml(acc.totpSecret) + '\')" class="btn btn-action-mini"' + busyBtnAttr + ' title="获取并复制当前 6 位 2FA 动态码">';
      html += '      <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>';
      html += '      <span>2FA</span>';
      html += '    </button>';
    }

    html += '    <button onclick="deleteGoogleAccount(\'' + _escapeGoogleHtml(acc.email) + '\')" class="btn btn-action-mini danger"' + busyBtnAttr + ' title="删除账号并清理本地 Profile">';
    html += '      <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>';
    html += '    </button>';
    html += '  </div>';
    html += '</td>';
    html += '</tr>';
  }

  tbody.innerHTML = html;
}

function openAddGoogleModal() {
  var modal = document.getElementById('modal-add-google');
  if (modal) modal.style.display = 'flex';
}

function closeAddGoogleModal() {
  var modal = document.getElementById('modal-add-google');
  if (modal) modal.style.display = 'none';
  var input = document.getElementById('cfg-google-data');
  if (input) input.value = '';
}

async function addGoogleAccounts() {
  var input = document.getElementById('cfg-google-data');
  if (!input || !input.value.trim()) {
    showToast('请输入有效的 Google 账号数据', 'warn');
    return;
  }

  try {
    var res = await window.go.main.App.AddGoogleAccounts(input.value);
    if (res.error) {
      showToast(res.error, 'error');
      return;
    }
    showToast('已添加 ' + res.added + ' 个，更新 ' + res.updated + ' 个', 'success');
    closeAddGoogleModal();
    loadGoogleAccounts();
  } catch (e) {
    showToast('添加失败: ' + e.message, 'error');
  }
}

async function importGoogleFile() {
  try {
    var filePath = await window.go.main.App.SelectGoogleFile();
    if (!filePath) return;
    var res = await window.go.main.App.ImportGoogleFile(filePath);
    if (res.error) {
      showToast(res.error, 'error');
      return;
    }
    showToast('导入成功: 新增 ' + res.added + ' 个，更新 ' + res.updated + ' 个', 'success');
    closeAddGoogleModal();
    loadGoogleAccounts();
  } catch (e) {
    showToast('导入失败: ' + e.message, 'error');
  }
}

async function deleteGoogleAccount(email) {
  if (!confirm('确定要删除该 Google 账号并清理其独立 Profile 吗？')) return;
  try {
    var res = await window.go.main.App.DeleteGoogleAccount(email);
    if (res.error) {
      showToast(res.error, 'error');
      return;
    }
    showToast('删除成功', 'success');
    loadGoogleAccounts();
  } catch (e) {
    showToast('删除失败: ' + e.message, 'error');
  }
}

async function clearAllGoogleAccounts() {
  if (!confirm('确定清空全部 Google 账号及其对应的 Profile 数据吗？此操作不可逆！')) return;
  try {
    var res = await window.go.main.App.ClearGoogleAccounts();
    if (res.error) {
      showToast(res.error, 'error');
      return;
    }
    showToast('已清空全部 Google 账号', 'success');
    loadGoogleAccounts();
  } catch (e) {
    showToast('清空失败: ' + e.message, 'error');
  }
}

async function openGoogleProfile(email) {
  if (isGoogleAccountBusy(email)) {
    showToast('账号 [' + email + '] 正在执行任务中，请稍后再操作环境', 'warning');
    return;
  }
  try {
    showToast('正在拉起独立 Profile 浏览器窗口...', 'info');
    var res = await window.go.main.App.OpenGoogleProfile(email);
    if (res && res.error) {
      showToast(res.error, 'error');
      return;
    }
    // 延迟 4 秒后自动同步一次状态
    setTimeout(function() {
      syncGoogleStatus();
    }, 4000);
  } catch (e) {
    showToast('打开环境失败: ' + e.message, 'error');
  }
}

async function authorizeKiroWithGoogle(email) {
  if (isGoogleAccountBusy(email)) {
    showToast('账号 [' + email + '] 正在执行任务中，请稍后再拉起授权', 'warning');
    return;
  }
  try {
    showToast('正在拉起独立 Profile 并直接进入 Kiro 官方 Google 授权...', 'info');
    var res = await window.go.main.App.AuthorizeKiroWithGoogle(email);
    if (res && res.error) {
      showToast(res.error, 'error');
      return;
    }
    showToast('已拉起浏览器，请在弹出窗口中完成 Kiro 授权确认', 'success');
    setTimeout(function() {
      syncGoogleStatus();
    }, 5000);
  } catch (e) {
    showToast('授权启动失败: ' + e.message, 'error');
  }
}

async function syncGoogleStatus() {
  try {
    var res = await window.go.main.App.SyncGoogleAccountsStatus();
    if (res && res.accounts) {
      _googleAccounts = res.accounts;
      renderGoogleAccounts(_googleAccounts);
      showToast('已完成本地 Profile 环境状态同步', 'success');
    }
  } catch (e) {
    console.error('同步状态失败:', e);
  }
}

async function copyGoogleTOTP(secret) {
  try {
    var res = await window.go.main.App.GetGoogleTOTP(secret);
    if (res.error) {
      showToast(res.error, 'error');
      return;
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(res.code);
      showToast('2FA 验证码已复制: ' + res.code, 'success');
    } else {
      prompt('当前 2FA 动态码:', res.code);
    }
  } catch (e) {
    showToast('获取 2FA 失败: ' + e.message, 'error');
  }
}

async function autoLoginGoogleAccount(email) {
  if (isGoogleAccountBusy(email)) {
    showToast('账号 [' + email + '] 正在执行任务中，请勿重复操作', 'warning');
    return;
  }
  setGoogleAccountBusy(email, '登录中');
  try {
    showToast('正在启动 Google 账号全自动登录...', 'info');
    var res = await window.go.main.App.AutoLoginGoogleAccount(email, false);
    if (res && res.error) {
      clearGoogleAccountBusy(email);
      showToast(res.error, 'error');
      return;
    }
    showToast('已启动全自动登录，Chrome 正在前台自动模拟击键输入...', 'success');
  } catch (e) {
    clearGoogleAccountBusy(email);
    showToast('启动自动登录失败: ' + e.message, 'error');
  }
}

async function batchAutoLoginGoogle() {
  if (!confirm('确定对所有未就绪的 Google 账号启动全自动登录吗？\n（系统将依次拉起独立环境并自动填入账号、密码、2FA及辅助邮箱）')) return;
  try {
    showToast('正在排队批量自动登录...', 'info');
    var res = await window.go.main.App.BatchAutoLoginGoogleAccounts(false);
    if (res && res.error) {
      showToast(res.error, 'warn');
      return;
    }
    showToast('已开始批量自动登录 ' + res.count + ' 个账号', 'success');
  } catch (e) {
    showToast('批量登录启动失败: ' + e.message, 'error');
  }
}

// ===== 订阅长效付款链接与仿真填卡 =====

// 复制付款链接
async function copyPaymentUrl(url) {
  if (!url) {
    showToast('该账号暂无付款链接', 'warn');
    return;
  }
  try {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(url);
      showToast('Stripe 结账付款链接已复制到剪贴板！', 'success');
    } else {
      prompt('请手动复制 Stripe 结账链接:', url);
    }
  } catch (e) {
    prompt('请手动复制 Stripe 结账链接:', url);
  }
}

// 在默认浏览器中打开付款链接
function openCurrentPaymentUrl() {
  var urlInput = document.getElementById('autofill-payment-url');
  var url = urlInput ? urlInput.value.trim() : '';
  if (!url) {
    showToast('请输入有效的付款链接', 'warn');
    return;
  }
  if (window.go && window.go.main && window.go.main.App && window.go.main.App.OpenURL) {
    window.go.main.App.OpenURL(url);
  } else {
    window.open(url, '_blank');
  }
}

// 单个账号生成订阅付款链接
async function generateGooglePaymentLink(email, planType) {
  if (isGoogleAccountBusy(email)) {
    showToast('账号 [' + email + '] 正在执行任务中，请勿重复操作', 'warning');
    return;
  }
  if (!planType) {
    var planEl = document.getElementById('google-target-plan');
    planType = planEl ? planEl.value : 'KIRO_PRO';
  }

  setGoogleAccountBusy(email, '生成链接中');
  try {
    showToast('正在拉起环境提取 [' + planType + '] 付款链接...', 'info');
    var res = await window.go.main.App.GenerateGoogleSubscriptionLink(email, planType);
    if (res && res.error) {
      clearGoogleAccountBusy(email);
      showToast(res.error, 'error');
      return;
    }
    showToast('已启动后台提取，提取成功后将自动保存并刷新', 'info');
  } catch (e) {
    clearGoogleAccountBusy(email);
    showToast('启动生成失败: ' + e.message, 'error');
  }
}

// 批量生成订阅付款链接
async function batchGenerateGooglePaymentLinks() {
  var planEl = document.getElementById('google-target-plan');
  var planType = planEl ? planEl.value : 'KIRO_PRO';

  var selected = getSelectedGoogleEmails();
  var targets = [];

  if (selected.length > 0) {
    targets = selected;
  } else {
    // 默认对所有已授权 Kiro 的账号生成
    var authorized = _googleAccounts.filter(function(a) { return a.status === 'authorized'; });
    if (authorized.length === 0) {
      showToast('未勾选账号，且当前没有已授权 Kiro 的账号', 'warn');
      return;
    }
    targets = authorized.map(function(a) { return a.email; });
  }

  var planText = planEl ? planEl.options[planEl.selectedIndex].text : planType;
  var confirmMsg = '确定为以下 ' + targets.length + ' 个账号批量生成 24h 免登录 Stripe 付款链接吗？\n' +
    '目标套餐：' + planText + '\n（系统将拉起独立环境进入设置页提取 Stripe Checkout Session）';

  if (!confirm(confirmMsg)) return;

  try {
    showToast('正在启动批量生成付款链接任务...', 'info');
    var res = await window.go.main.App.BatchGenerateGoogleSubscriptionLinks(targets, planType, 2);
    if (res && res.error) {
      showToast(res.error, 'error');
      return;
    }
    showToast('批量生成任务已启动 (并发: 2)，进度将实时推送', 'success');
  } catch (e) {
    showToast('启动批量生成失败: ' + e.message, 'error');
  }
}

// Stripe 仿真卡片动态预览与格式化
var _stripeAutofillListenersBound = false;

function updateStripeCardPreview() {
  var numEl = document.getElementById('autofill-card-number');
  var expEl = document.getElementById('autofill-card-expiry');
  var nameEl = document.getElementById('autofill-billing-name');

  var prevNum = document.getElementById('stripe-card-preview-number');
  var prevExp = document.getElementById('stripe-card-preview-expiry');
  var prevName = document.getElementById('stripe-card-preview-name');
  var brandLabel = document.getElementById('stripe-card-brand-label');

  var numVal = numEl ? numEl.value.trim() : '';
  var expVal = expEl ? expEl.value.trim() : '';
  var nameVal = nameEl ? nameEl.value.trim() : '';

  if (prevNum) {
    prevNum.innerText = numVal || '•••• •••• •••• ••••';
  }
  if (prevExp) {
    prevExp.innerText = expVal || '12/28';
  }
  if (prevName) {
    prevName.innerText = (nameVal || 'JOHN DOE').toUpperCase();
  }

  if (brandLabel) {
    var rawNum = numVal.replace(/\s+/g, '');
    if (/^4/.test(rawNum)) {
      brandLabel.innerText = 'VISA';
    } else if (/^5[1-5]/.test(rawNum) || /^2[2-7]/.test(rawNum)) {
      brandLabel.innerText = 'MASTERCARD';
    } else if (/^3[47]/.test(rawNum)) {
      brandLabel.innerText = 'AMEX';
    } else if (/^6(?:011|5)/.test(rawNum)) {
      brandLabel.innerText = 'DISCOVER';
    } else {
      brandLabel.innerText = 'CARD PREVIEW';
    }
  }
}

function bindStripeAutofillListeners() {
  if (_stripeAutofillListenersBound) return;
  _stripeAutofillListenersBound = true;

  var numEl = document.getElementById('autofill-card-number');
  var expEl = document.getElementById('autofill-card-expiry');
  var nameEl = document.getElementById('autofill-billing-name');

  if (numEl) {
    numEl.addEventListener('input', function(e) {
      var raw = e.target.value.replace(/\D/g, '').substring(0, 19);
      var formatted = raw.replace(/(\d{4})(?=\d)/g, '$1 ').trim();
      e.target.value = formatted;
      updateStripeCardPreview();
    });
  }

  if (expEl) {
    expEl.addEventListener('input', function(e) {
      var raw = e.target.value.replace(/\D/g, '').substring(0, 4);
      if (raw.length >= 3) {
        e.target.value = raw.substring(0, 2) + '/' + raw.substring(2);
      } else {
        e.target.value = raw;
      }
      updateStripeCardPreview();
    });
  }

  if (nameEl) {
    nameEl.addEventListener('input', function() {
      updateStripeCardPreview();
    });
  }
}

// 打开仿真自动填卡模态框
function openAutofillStripeModal(email, paymentUrl) {
  var modal = document.getElementById('modal-stripe-autofill');
  if (!modal) return;

  bindStripeAutofillListeners();
  updateStripeCardPreview();

  var urlInput = document.getElementById('autofill-payment-url');
  var openBtn = document.getElementById('autofill-open-browser-btn');
  var progressBox = document.getElementById('autofill-progress-box');
  var progressText = document.getElementById('autofill-progress-text');
  var startBtn = document.getElementById('btn-start-stripe-autofill');

  if (progressBox) progressBox.style.display = 'none';
  if (progressText) progressText.innerText = '';
  if (startBtn) startBtn.disabled = false;

  var urlToFill = paymentUrl || '';
  if (!urlToFill && email) {
    var acc = _googleAccounts.find(function(a) { return a.email === email; });
    if (acc && acc.paymentUrl) urlToFill = acc.paymentUrl;
  }

  if (urlInput) {
    urlInput.value = urlToFill;
    urlInput.oninput = function() {
      if (openBtn) openBtn.style.display = urlInput.value.trim() ? 'inline' : 'none';
    };
  }

  if (openBtn) {
    openBtn.style.display = urlToFill ? 'inline' : 'none';
  }

  modal.style.display = 'flex';
}

// 关闭仿真自动填卡模态框
function closeAutofillStripeModal() {
  var modal = document.getElementById('modal-stripe-autofill');
  if (modal) modal.style.display = 'none';
}

// 一键填充测试卡数据
function fillTestCardData() {
  var setVal = function(id, val) {
    var el = document.getElementById(id);
    if (el) el.value = val;
  };
  setVal('autofill-card-number', '4242 4242 4242 4242');
  setVal('autofill-card-expiry', '12/28');
  setVal('autofill-card-cvc', '123');
  setVal('autofill-billing-name', 'John Doe');
  setVal('autofill-billing-country', 'US');
  setVal('autofill-billing-postal', '90001');
  setVal('autofill-billing-address', '123 Main Street');
  setVal('autofill-billing-city', 'Los Angeles');
  setVal('autofill-billing-state', 'CA');
  updateStripeCardPreview();
  showToast('已填充测试信用卡与账单示例数据', 'info');
}

// 提交仿真自动填卡
async function submitAutofillStripe() {
  var getVal = function(id) {
    var el = document.getElementById(id);
    return el ? el.value.trim() : '';
  };

  var paymentUrl = getVal('autofill-payment-url');
  if (!paymentUrl) {
    showToast('请输入或选择 Stripe 付款链接', 'warn');
    return;
  }

  var cardNumber = getVal('autofill-card-number');
  if (!cardNumber) {
    showToast('请输入卡号', 'warn');
    return;
  }

  var expiry = getVal('autofill-card-expiry');
  var expMonth = '12';
  var expYear = '28';
  if (expiry.indexOf('/') !== -1) {
    var parts = expiry.split('/');
    expMonth = parts[0].trim();
    expYear = parts[1].trim();
  }

  var cardInfo = {
    cardNumber: cardNumber,
    expMonth: expMonth,
    expYear: expYear,
    cvc: getVal('autofill-card-cvc'),
    name: getVal('autofill-billing-name'),
    country: getVal('autofill-billing-country') || 'US',
    postalCode: getVal('autofill-billing-postal'),
    addressLine1: getVal('autofill-billing-address'),
    city: getVal('autofill-billing-city'),
    state: getVal('autofill-billing-state')
  };

  var autoSubmit = false;
  var autoSubmitEl = document.getElementById('autofill-auto-submit');
  if (autoSubmitEl && autoSubmitEl.checked) {
    autoSubmit = true;
  }

  var progressBox = document.getElementById('autofill-progress-box');
  var progressText = document.getElementById('autofill-progress-text');
  var startBtn = document.getElementById('btn-start-stripe-autofill');

  if (progressBox) progressBox.style.display = 'block';
  if (progressText) progressText.innerText = '正在拉起浏览器打开结账页面...';
  if (startBtn) startBtn.disabled = true;

  try {
    var res = await window.go.main.App.AutofillStripePayment(paymentUrl, cardInfo, autoSubmit);
    if (res && res.error) {
      showToast(res.error, 'error');
      if (progressBox) progressBox.style.display = 'none';
      if (startBtn) startBtn.disabled = false;
      return;
    }
    showToast('已启动 Stripe 自动填卡，请观察浏览器操作', 'info');
  } catch (e) {
    showToast('启动填卡失败: ' + e.message, 'error');
    if (progressBox) progressBox.style.display = 'none';
    if (startBtn) startBtn.disabled = false;
  }
}

// 注册 Wails 后端进度推送监听
if (window.runtime && window.runtime.EventsOn) {
  // 登录进度
  window.runtime.EventsOn('google_login_progress', function(data) {
    if (!data) return;
    var prefix = data.email ? '[' + data.email.split('@')[0] + '] ' : '';
    var msg = prefix + (data.message || '');
    var toastType = 'info';
    if (data.status === 'error') toastType = 'error';
    else if (data.status === 'logged_in' || data.status === 'authorized') toastType = 'success';

    showToast(msg, toastType);
    if (data.status === 'logged_in' || data.status === 'authorized' || data.status === 'error') {
      if (data.email) clearGoogleAccountBusy(data.email);
      loadGoogleAccounts();
    }
  });

  // 付款链接生成进度
  window.runtime.EventsOn('google_payment_progress', function(data) {
    if (!data) return;
    var prefix = data.email ? '[' + data.email.split('@')[0] + '] ' : '';
    var msg = prefix + (data.message || '');
    var toastType = 'info';
    if (data.status === 'error') toastType = 'error';
    else if (data.status === 'generated' || data.status === 'batch_done') toastType = 'success';

    showToast(msg, toastType);
    if (data.status === 'generated' || data.status === 'error' || data.status === 'batch_done') {
      if (data.email) clearGoogleAccountBusy(data.email);
      if (data.status === 'batch_done') {
        _activeGoogleTasks.clear();
      }
      loadGoogleAccounts();
    }
  });

  // Stripe 自动填卡进度
  window.runtime.EventsOn('stripe_autofill_progress', function(data) {
    if (!data) return;
    var msg = data.message || '';
    var progressBox = document.getElementById('autofill-progress-box');
    var progressText = document.getElementById('autofill-progress-text');
    var startBtn = document.getElementById('btn-start-stripe-autofill');

    if (progressBox) progressBox.style.display = 'block';
    if (progressText) progressText.innerText = msg;

    if (data.status === 'error') {
      showToast(msg, 'error');
      if (startBtn) startBtn.disabled = false;
    } else if (data.status === 'done') {
      showToast(msg, 'success');
      if (startBtn) startBtn.disabled = false;
    }
  });

  // 单账号 2FA 进度
  window.runtime.EventsOn('google_2fa_progress', function(data) {
    if (!data) return;
    var consoleBox = document.getElementById('manage-2fa-console');
    var btnAuto = document.getElementById('btn-auto-change-2fa');
    var prefix = data.email ? '[' + data.email.split('@')[0] + '] ' : '';
    var text = prefix + (data.message || '');

    if (consoleBox) {
      consoleBox.style.display = 'block';
      var color = '#e2e8f0';
      if (data.status === 'success') color = '#34d399';
      else if (data.status === 'error') color = '#f87171';
      else if (data.status === 'in_progress') color = '#60a5fa';

      var time = new Date().toLocaleTimeString();
      var line = '<div style="color:' + color + ';margin-bottom:3px;"><span style="opacity:0.6;">[' + time + ']</span> ' + _escapeGoogleHtml(text) + '</div>';
      consoleBox.innerHTML += line;
      consoleBox.scrollTop = consoleBox.scrollHeight;
    }

    if (data.status === 'success') {
      if (data.email) clearGoogleAccountBusy(data.email);
      showToast(text, 'success');
      if (btnAuto) btnAuto.disabled = false;
      loadGoogleAccounts().then(function() {
        if (_currentManageEmail) {
          var acc = (_googleAccounts || []).find(function(a) { return a.email === _currentManageEmail; });
          if (acc) {
            var inputTotp = document.getElementById('manage-input-totp');
            if (inputTotp) inputTotp.value = acc.totpSecret || '';
            var backupRow = document.getElementById('manage-backup-key-row');
            var backupVal = document.getElementById('manage-backup-key-val');
            if (backupRow && backupVal && acc.backupTotpSecret) {
              backupRow.style.display = 'block';
              backupVal.innerText = acc.backupTotpSecret;
            }
            onManageTOTPInput();
          }
        }
      });
    } else if (data.status === 'error') {
      if (data.email) clearGoogleAccountBusy(data.email);
      showToast(text, 'error');
      if (btnAuto) btnAuto.disabled = false;
    }
  });

  // 批量 2FA 进度
  window.runtime.EventsOn('google_batch_2fa_progress', function(data) {
    if (!data) return;
    var consoleBox = document.getElementById('batch-2fa-console');
    var btnStart = document.getElementById('btn-start-batch-2fa');
    var btnExport = document.getElementById('btn-export-batch-2fa');
    var prefix = data.email ? '[' + data.email.split('@')[0] + '] ' : '';
    var text = prefix + (data.message || '');

    if (consoleBox) {
      consoleBox.style.display = 'block';
      var color = '#e2e8f0';
      if (data.status === 'success' || data.status === 'batch_done') color = '#34d399';
      else if (data.status === 'error') color = '#f87171';
      else if (data.status === 'in_progress') color = '#60a5fa';

      var time = new Date().toLocaleTimeString();
      var line = '<div style="color:' + color + ';margin-bottom:3px;"><span style="opacity:0.6;">[' + time + ']</span> ' + _escapeGoogleHtml(text) + '</div>';
      consoleBox.innerHTML += line;
      consoleBox.scrollTop = consoleBox.scrollHeight;
    }

    if (data.status === 'batch_done') {
      _activeGoogleTasks.clear();
      showToast('批量修改 2FA 全部执行完成！', 'success');
      if (btnStart) btnStart.disabled = false;
      if (btnExport) btnExport.style.display = 'inline-flex';
      loadGoogleAccounts();
    } else if (data.status === 'error' && !data.email) {
      _activeGoogleTasks.clear();
      showToast(text, 'error');
      if (btnStart) btnStart.disabled = false;
    } else if (data.status === 'item_success' || data.status === 'error') {
      if (data.email) clearGoogleAccountBusy(data.email);
      loadGoogleAccounts();
    } else if (data.status === 'success') {
      if (data.email) clearGoogleAccountBusy(data.email);
      loadGoogleAccounts();
    }
  });

  // Kiro 会话凭据提取与刷新进度
  window.runtime.EventsOn('google_kiro_creds_progress', function(data) {
    if (!data) return;
    var consoleBox = document.getElementById('manage-kiro-console');
    var btnExtract = document.getElementById('btn-extract-kiro-creds');
    var prefix = data.email ? '[' + data.email.split('@')[0] + '] ' : '';
    var text = prefix + (data.message || '');

    if (consoleBox) {
      consoleBox.style.display = 'block';
      var color = '#e2e8f0';
      if (data.status === 'success') color = '#34d399';
      else if (data.status === 'error') color = '#f87171';
      else if (data.status === 'in_progress') color = '#60a5fa';

      var time = new Date().toLocaleTimeString();
      var line = '<div style="color:' + color + ';margin-bottom:3px;"><span style="opacity:0.6;">[' + time + ']</span> ' + _escapeGoogleHtml(text) + '</div>';
      consoleBox.innerHTML += line;
      consoleBox.scrollTop = consoleBox.scrollHeight;
    }

    if (data.status === 'success') {
      if (data.email) clearGoogleAccountBusy(data.email);
      showToast(text, 'success');
      if (btnExtract) btnExtract.disabled = false;
      loadGoogleAccounts().then(function() {
        if (_currentManageEmail) {
          loadManageKiroCredentials(_currentManageEmail);
        }
      });
    } else if (data.status === 'error') {
      if (data.email) clearGoogleAccountBusy(data.email);
      showToast(text, 'error');
      if (btnExtract) btnExtract.disabled = false;
    }
  });
}

// ==================== Google 账号管理与 2FA 修改 ====================

var _currentManageEmail = '';
var _manageCountdownTimer = null;
var _manageRemainingSeconds = 0;

function openGoogleManageModal(email, initialTab) {
  _currentManageEmail = email;
  var acc = (_googleAccounts || []).find(function(a) { return a.email === email; });
  if (!acc) return;

  var titleEmail = document.getElementById('manage-modal-email');
  if (titleEmail) titleEmail.innerText = email;

  var inputEmail = document.getElementById('manage-input-email');
  if (inputEmail) inputEmail.value = email;

  var inputPwd = document.getElementById('manage-input-password');
  if (inputPwd) inputPwd.value = acc.password || '';

  var inputRec = document.getElementById('manage-input-recovery');
  if (inputRec) inputRec.value = acc.recovery || acc.recoveryEmail || '';

  var inputProxy = document.getElementById('manage-input-proxy');
  if (inputProxy) inputProxy.value = acc.proxy || '';

  var inputTotp = document.getElementById('manage-input-totp');
  if (inputTotp) inputTotp.value = acc.totpSecret || '';

  var backupRow = document.getElementById('manage-backup-key-row');
  var backupVal = document.getElementById('manage-backup-key-val');
  if (backupRow && backupVal) {
    if (acc.backupTotpSecret) {
      backupRow.style.display = 'block';
      backupVal.innerText = acc.backupTotpSecret;
    } else {
      backupRow.style.display = 'none';
      backupVal.innerText = '';
    }
  }

  var envDir = document.getElementById('manage-env-profile-dir');
  if (envDir) envDir.innerText = acc.profileDir || ('~/.kirox/profiles/' + email.replace(/[^a-zA-Z0-9]/g, '_'));

  var envStatus = document.getElementById('manage-env-status');
  if (envStatus) {
    var statusMap = {
      'logged_in': '🟢 已登录 (Local Profile 就绪)',
      'authorized': '🟣 已授权 (Kiro 授权通过)',
      'uninit': '⚪ 未初始化/未登录',
      'error': '🔴 状态异常'
    };
    envStatus.innerText = statusMap[acc.status] || (acc.status || '未登录');
  }

  // 重置终端输出和按钮
  var consoleBox = document.getElementById('manage-2fa-console');
  if (consoleBox) {
    consoleBox.style.display = 'none';
    consoleBox.innerHTML = '';
  }
  var kiroConsoleBox = document.getElementById('manage-kiro-console');
  if (kiroConsoleBox) {
    kiroConsoleBox.style.display = 'none';
    kiroConsoleBox.innerHTML = '';
  }
  var btnAuto = document.getElementById('btn-auto-change-2fa');
  if (btnAuto) btnAuto.disabled = false;
  var btnExtract = document.getElementById('btn-extract-kiro-creds');
  if (btnExtract) btnExtract.disabled = false;

  switchManageSubTab(initialTab || '2fa');
  onManageTOTPInput();

  var modal = document.getElementById('modal-google-account-manage');
  if (modal) modal.style.display = 'flex';
}

function closeGoogleManageModal() {
  if (_manageCountdownTimer) {
    clearInterval(_manageCountdownTimer);
    _manageCountdownTimer = null;
  }
  var modal = document.getElementById('modal-google-account-manage');
  if (modal) modal.style.display = 'none';
}

function switchManageSubTab(tab) {
  var tabs = ['2fa', 'creds', 'env', 'kiro'];
  for (var i = 0; i < tabs.length; i++) {
    var t = tabs[i];
    var btn = document.getElementById('btn-subtab-' + t);
    var pane = document.getElementById('manage-pane-' + t);
    if (btn) {
      if (t === tab) btn.classList.add('active');
      else btn.classList.remove('active');
    }
    if (pane) {
      pane.style.display = (t === tab) ? 'block' : 'none';
    }
  }
  if (tab === 'kiro' && _currentManageEmail) {
    loadManageKiroCredentials(_currentManageEmail);
  }
}

async function onManageTOTPInput() {
  if (_manageCountdownTimer) {
    clearInterval(_manageCountdownTimer);
    _manageCountdownTimer = null;
  }

  var inputTotp = document.getElementById('manage-input-totp');
  var codeEl = document.getElementById('manage-totp-live-code');
  var countEl = document.getElementById('manage-totp-countdown');
  if (!inputTotp || !codeEl || !countEl) return;

  var secret = inputTotp.value.trim().replace(/\s+/g, '');
  if (!secret) {
    codeEl.innerText = '------';
    countEl.innerText = '未设置密钥';
    return;
  }

  try {
    var res = await window.go.main.App.VerifyTOTPSecret(secret);
    if (res && (res.valid || res.success) && res.code) {
      codeEl.innerText = res.code;
      _manageRemainingSeconds = res.remaining || 30;
      countEl.innerText = '剩余 ' + _manageRemainingSeconds + 's';

      _manageCountdownTimer = setInterval(function() {
        _manageRemainingSeconds--;
        if (_manageRemainingSeconds <= 0) {
          clearInterval(_manageCountdownTimer);
          _manageCountdownTimer = null;
          onManageTOTPInput();
        } else {
          if (countEl) countEl.innerText = '剩余 ' + _manageRemainingSeconds + 's';
        }
      }, 1000);
    } else {
      codeEl.innerText = '无效密钥';
      countEl.innerText = (res && res.error) ? '格式错误' : '格式错误';
    }
  } catch (e) {
    codeEl.innerText = '算码失败';
    countEl.innerText = 'ERR';
  }
}

function copyCurrentManageTOTP() {
  var codeEl = document.getElementById('manage-totp-live-code');
  if (!codeEl) return;
  var code = codeEl.innerText.trim();
  if (!code || code === '------' || code === '无效密钥' || code === '算码失败') {
    showToast('暂无有效的 2FA 动态码可供复制', 'warning');
    return;
  }
  navigator.clipboard.writeText(code).then(function() {
    showToast('2FA 动态码 [' + code + '] 已复制到剪贴板', 'success');
  }).catch(function() {
    showToast('复制失败，请手动复制', 'error');
  });
}

async function saveGoogle2FAManual() {
  if (!_currentManageEmail) return;
  var inputTotp = document.getElementById('manage-input-totp');
  var newSecret = inputTotp ? inputTotp.value.trim().replace(/\s+/g, '') : '';
  if (!newSecret) {
    showToast('2FA 密钥不能为空（留空保存会误改其他字段）', 'warning');
    return;
  }

  if (newSecret) {
    var res = await window.go.main.App.VerifyTOTPSecret(newSecret);
    if (!res || (!res.valid && !res.success) || !res.code) {
      showToast('输入的 2FA 密钥不是有效的 Base32 格式，请检查！', 'error');
      return;
    }
  }

  try {
    var r = await window.go.main.App.UpdateGoogleAccountManual(_currentManageEmail, '', '', newSecret, '');
    if (r && r.error) {
      showToast('保存 2FA 失败: ' + r.error, 'error');
      return;
    }
    showToast('Google 账号 2FA 密钥已成功保存并备份旧密钥', 'success');
    await loadGoogleAccounts();
    var acc = (_googleAccounts || []).find(function(a) { return a.email === _currentManageEmail; });
    if (acc) {
      var backupRow = document.getElementById('manage-backup-key-row');
      var backupVal = document.getElementById('manage-backup-key-val');
      if (backupRow && backupVal && acc.backupTotpSecret) {
        backupRow.style.display = 'block';
        backupVal.innerText = acc.backupTotpSecret;
      }
    }
  } catch (e) {
    showToast('保存 2FA 失败: ' + e.message, 'error');
  }
}

async function saveGoogleCredentialsManual() {
  if (!_currentManageEmail) return;
  var pwd = document.getElementById('manage-input-password').value.trim();
  var rec = document.getElementById('manage-input-recovery').value.trim();
  var prx = document.getElementById('manage-input-proxy').value.trim();

  try {
    var r = await window.go.main.App.UpdateGoogleAccountManual(_currentManageEmail, pwd, rec, '', prx);
    if (r && r.error) {
      showToast('保存资料失败: ' + r.error, 'error');
      return;
    }
    showToast('账号资料及代理已成功保存', 'success');
    await loadGoogleAccounts();
    closeGoogleManageModal();
  } catch (e) {
    showToast('保存资料失败: ' + e.message, 'error');
  }
}

async function autoChangeCurrentGoogle2FA() {
  if (!_currentManageEmail) return;
  if (isGoogleAccountBusy(_currentManageEmail)) {
    showToast('账号 [' + _currentManageEmail + '] 正在执行任务中，请勿重复操作', 'warning');
    return;
  }
  setGoogleAccountBusy(_currentManageEmail, '改2FA中');
  var btn = document.getElementById('btn-auto-change-2fa');
  var consoleBox = document.getElementById('manage-2fa-console');
  if (consoleBox) {
    consoleBox.style.display = 'block';
    consoleBox.innerHTML = '<div style="color:#60a5fa;">[系统] 正在准备拉起专属 Chrome Profile 连接谷歌安全中心...</div>';
  }
  if (btn) btn.disabled = true;

  try {
    var res = await window.go.main.App.ChangeGoogle2FAAuto(_currentManageEmail);
    if (res && res.error) {
      clearGoogleAccountBusy(_currentManageEmail);
      showToast('自动修改 2FA 失败: ' + res.error, 'error');
      if (btn) btn.disabled = false;
      return;
    }
    showToast('修改 2FA 任务已发起，请观察操作进程', 'info');
  } catch (e) {
    clearGoogleAccountBusy(_currentManageEmail);
    showToast('发起任务失败: ' + e.message, 'error');
    if (btn) btn.disabled = false;
  }
}

async function openCurrentGoogleSecurity() {
  if (!_currentManageEmail) return;
  if (isGoogleAccountBusy(_currentManageEmail)) {
    showToast('账号 [' + _currentManageEmail + '] 正在执行自动化任务，请稍后再操作安全中心', 'warning');
    return;
  }
  try {
    var res = await window.go.main.App.OpenGoogleSecurityCenter(_currentManageEmail);
    if (res && res.error) {
      showToast(res.error, 'error');
    } else {
      showToast('已拉起 Chrome 浏览器直达两步验证管理页面', 'info');
    }
  } catch (e) {
    showToast('拉起浏览器失败: ' + e.message, 'error');
  }
}

function openCurrentGoogleProfile() {
  if (!_currentManageEmail) return;
  openGoogleProfile(_currentManageEmail);
}

async function syncCurrentGoogleStatus() {
  await loadGoogleAccounts();
  if (_currentManageEmail) {
    var acc = (_googleAccounts || []).find(function(a) { return a.email === _currentManageEmail; });
    if (acc) {
      var envStatus = document.getElementById('manage-env-status');
      if (envStatus) {
        var statusMap = {
          'logged_in': '🟢 已登录 (Local Profile 就绪)',
          'authorized': '🟣 已授权 (Kiro 授权通过)',
          'uninit': '⚪ 未初始化/未登录',
          'error': '🔴 状态异常'
        };
        envStatus.innerText = statusMap[acc.status] || (acc.status || '未登录');
      }
    }
  }
  showToast('环境状态已同步', 'info');
}

function openBatchChangeGoogle2FAModal() {
  var count = _selectedGoogleEmails ? _selectedGoogleEmails.size : 0;
  if (count === 0) {
    showToast('请先勾选需要修改 2FA 的 Google 账号', 'warning');
    return;
  }
  var countEl = document.getElementById('batch-2fa-count');
  if (countEl) countEl.innerText = count;

  var consoleBox = document.getElementById('batch-2fa-console');
  if (consoleBox) {
    consoleBox.style.display = 'none';
    consoleBox.innerHTML = '';
  }
  var btnStart = document.getElementById('btn-start-batch-2fa');
  if (btnStart) btnStart.disabled = false;
  var btnExport = document.getElementById('btn-export-batch-2fa');
  if (btnExport) btnExport.style.display = 'none';

  var modal = document.getElementById('modal-batch-google-2fa');
  if (modal) modal.style.display = 'flex';
}

function closeBatchChangeGoogle2FAModal() {
  var modal = document.getElementById('modal-batch-google-2fa');
  if (modal) modal.style.display = 'none';
}

async function runBatchChangeGoogle2FA() {
  var emails = Array.from(_selectedGoogleEmails || []);
  if (emails.length === 0) {
    showToast('请先勾选需要修改 2FA 的 Google 账号', 'warning');
    return;
  }

  var btnStart = document.getElementById('btn-start-batch-2fa');
  var consoleBox = document.getElementById('batch-2fa-console');
  if (btnStart) btnStart.disabled = true;
  if (consoleBox) {
    consoleBox.style.display = 'block';
    consoleBox.innerHTML = '<div style="color:#60a5fa;">[系统] 正在启动批量修改 2FA 队列，共计 ' + emails.length + ' 个账号...</div>';
  }

  try {
    var res = await window.go.main.App.BatchChangeGoogle2FAAuto(emails);
    if (res && res.error) {
      showToast('启动批量改 2FA 失败: ' + res.error, 'error');
      if (btnStart) btnStart.disabled = false;
      return;
    }
    showToast('批量修改 2FA 队列已启动，请观察实时进度', 'info');
  } catch (e) {
    showToast('启动批量任务失败: ' + e.message, 'error');
    if (btnStart) btnStart.disabled = false;
  }
}

function exportUpdatedGoogleAccounts() {
  if (!_googleAccounts || _googleAccounts.length === 0) {
    showToast('暂无 Google 账号可供导出', 'warning');
    return;
  }

  var lines = [];
  for (var i = 0; i < _googleAccounts.length; i++) {
    var a = _googleAccounts[i];
    var line = (a.email || '') + '----' + (a.password || '') + '----' + (a.recovery || a.recoveryEmail || '') + '----' + (a.totpSecret || '');
    if (a.proxy) {
      line += '----' + a.proxy;
    }
    lines.push(line);
  }

  var blob = new Blob([lines.join('\r\n')], { type: 'text/plain;charset=utf-8' });
  var url = URL.createObjectURL(blob);
  var link = document.createElement('a');
  link.href = url;
  var now = new Date();
  var ts = now.getFullYear() + '-' + String(now.getMonth()+1).padStart(2, '0') + '-' + String(now.getDate()).padStart(2, '0') + '_' + String(now.getHours()).padStart(2, '0') + String(now.getMinutes()).padStart(2, '0');
  link.download = 'google_accounts_2fa_updated_' + ts + '.txt';
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
  showToast('已导出 ' + lines.length + ' 个 Google 账号最新数据', 'success');
}

// ==================== Kiro 凭据管理 (JSON 生成、复制、导出) ====================

async function loadManageKiroCredentials(email, format) {
  if (!email) email = _currentManageEmail;
  if (!email) return;

  if (!format) {
    var checkedRadio = document.querySelector('input[name="kiro-json-format"]:checked');
    format = checkedRadio ? checkedRadio.value : 'kam';
  }

  var dot = document.getElementById('manage-kiro-status-dot');
  var badge = document.getElementById('manage-kiro-status-badge');
  var uidEl = document.getElementById('manage-kiro-meta-uid');
  var expEl = document.getElementById('manage-kiro-meta-expiry');
  var arnEl = document.getElementById('manage-kiro-meta-arn');
  var previewEl = document.getElementById('manage-kiro-json-preview');

  try {
    var res = await window.go.main.App.GetGoogleKiroCredentials(email, format);
    var hasCred = !!(res && (res.hasCredentials || res.hasCreds || res.refreshToken || res.accessToken));
    if (!hasCred) {
      if (dot) dot.style.background = '#94a3b8';
      if (badge) {
        badge.innerText = '未提取';
        badge.style.color = 'var(--text-muted)';
        badge.style.background = 'var(--bg-card)';
        badge.style.borderColor = 'var(--border)';
      }
      if (uidEl) uidEl.innerText = '-';
      if (expEl) expEl.innerText = '-';
      if (arnEl) arnEl.innerText = '-';
      if (previewEl) {
        previewEl.value = '';
        previewEl.placeholder = '// 该账号尚未提取 Kiro 会话凭据，请点击右上角【提取/刷新凭据】\n// 提示：提取需要该账号已完成 Google 登录或已授权 Kiro。';
      }
      return;
    }

    // 凭据已就绪
    if (dot) dot.style.background = '#10b981';
    if (badge) {
      badge.innerText = '已就绪 (有效)';
      badge.style.color = '#059669';
      badge.style.background = 'rgba(16,185,129,0.12)';
      badge.style.borderColor = 'rgba(16,185,129,0.3)';
    }
    if (uidEl) uidEl.innerText = res.userId || '已绑定';
    if (expEl) expEl.innerText = res.expiresAt || '正常 (可自动续期)';
    if (arnEl) arnEl.innerText = res.profileArn || '-';
    if (previewEl) {
      previewEl.value = res.json || '';
    }
  } catch (e) {
    console.error('获取 Kiro 凭据失败:', e);
    if (previewEl) previewEl.value = '// 获取凭据异常: ' + e.message;
  }
}

function onChangeKiroFormat() {
  if (_currentManageEmail) {
    loadManageKiroCredentials(_currentManageEmail);
  }
}

async function extractCurrentGoogleKiroCreds() {
  if (!_currentManageEmail) return;
  if (isGoogleAccountBusy(_currentManageEmail)) {
    showToast('账号 [' + _currentManageEmail + '] 正在执行任务中，请勿重复操作', 'warning');
    return;
  }
  setGoogleAccountBusy(_currentManageEmail, '提取凭据中');
  var btn = document.getElementById('btn-extract-kiro-creds');
  var consoleBox = document.getElementById('manage-kiro-console');
  if (consoleBox) {
    consoleBox.style.display = 'block';
    consoleBox.innerHTML = '<div style="color:#60a5fa;margin-bottom:3px;">[' + new Date().toLocaleTimeString() + '] 正在启动独立 Profile 会话并提取 Kiro 凭据...</div>';
  }
  if (btn) btn.disabled = true;

  try {
    var res = await window.go.main.App.ExtractGoogleKiroCredentials(_currentManageEmail);
    clearGoogleAccountBusy(_currentManageEmail);
    if (res && res.error) {
      showToast(res.error, 'error');
      if (btn) btn.disabled = false;
      return;
    }
    showToast('Kiro 凭据提取成功！', 'success');
    if (btn) btn.disabled = false;
    await loadGoogleAccounts();
    loadManageKiroCredentials(_currentManageEmail);
  } catch (e) {
    clearGoogleAccountBusy(_currentManageEmail);
    showToast('提取失败: ' + e.message, 'error');
    if (btn) btn.disabled = false;
  }
}

function copyCurrentKiroJSON() {
  var previewEl = document.getElementById('manage-kiro-json-preview');
  if (!previewEl || !previewEl.value.trim()) {
    showToast('暂无有效的 Kiro 凭据 JSON 可供复制，请先点击【提取/刷新凭据】', 'warning');
    return;
  }
  var val = previewEl.value.trim();
  navigator.clipboard.writeText(val).then(function() {
    showToast('Kiro 凭据 JSON 已成功复制到剪贴板', 'success');
  }).catch(function() {
    showToast('复制失败，请手动全选复制', 'error');
  });
}

function downloadCurrentKiroJSON() {
  var previewEl = document.getElementById('manage-kiro-json-preview');
  if (!previewEl || !previewEl.value.trim()) {
    showToast('暂无有效的 Kiro 凭据 JSON 可供下载，请先点击【提取/刷新凭据】', 'warning');
    return;
  }
  var checkedRadio = document.querySelector('input[name="kiro-json-format"]:checked');
  var format = checkedRadio ? checkedRadio.value : 'kam';
  var safeEmail = (_currentManageEmail || 'account').replace(/[^a-zA-Z0-9]/g, '_');
  var filename = (format === 'ide') ? 'kiro-auth-token.json' : ('kiro_kam_' + safeEmail + '.json');

  var blob = new Blob([previewEl.value.trim()], { type: 'application/json;charset=utf-8' });
  var url = URL.createObjectURL(blob);
  var link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
  showToast('凭据文件已下载: ' + filename, 'success');
}

async function exportGoogleKiroCredentialsBatch() {
  if (!_googleAccounts || _googleAccounts.length === 0) {
    showToast('当前暂无 Google 账号', 'warning');
    return;
  }

  var emails = getSelectedGoogleEmails();
  var isAll = false;
  if (emails.length === 0) {
    isAll = true;
    emails = _googleAccounts.map(function(a) { return a.email; });
  }

  var msg = isAll 
    ? '确定导出全部 ' + emails.length + ' 个 Google 账号的 Kiro 凭据 (KAM 格式) 吗？'
    : '确定导出所勾选的 ' + emails.length + ' 个 Google 账号的 Kiro 凭据 (KAM 格式) 吗？';

  if (!confirm(msg)) return;

  try {
    var res = await window.go.main.App.BatchExportGoogleKiroCredentials(emails, 'kam');
    if (!res || !res.success) {
      showToast((res && res.error) || '导出失败', 'error');
      return;
    }
    if (res.count === 0) {
      showToast('所选账号中均未找到有效的 Kiro 凭据，请先提取或授权', 'warning');
      return;
    }

    if (res.cancelled) {
      showToast('已取消导出', 'warning');
      return;
    }
    // 后端已通过原生保存对话框写入磁盘，前端不再二次下载
    showToast('成功导出 ' + res.count + ' 个账号的 Kiro 凭据 (文件: ' + (res.path || '') + ')', 'success');
  } catch (e) {
    showToast('批量导出失败: ' + e.message, 'error');
  }
}

// 挂载至 window 以确保 HTML onclick 可直接访问
window.openGoogleManageModal = openGoogleManageModal;
window.closeGoogleManageModal = closeGoogleManageModal;
window.switchManageSubTab = switchManageSubTab;
window.onManageTOTPInput = onManageTOTPInput;
window.copyCurrentManageTOTP = copyCurrentManageTOTP;
window.saveGoogle2FAManual = saveGoogle2FAManual;
window.saveGoogleCredentialsManual = saveGoogleCredentialsManual;
window.autoChangeCurrentGoogle2FA = autoChangeCurrentGoogle2FA;
window.openCurrentGoogleSecurity = openCurrentGoogleSecurity;
window.openCurrentGoogleProfile = openCurrentGoogleProfile;
window.syncCurrentGoogleStatus = syncCurrentGoogleStatus;
window.openBatchChangeGoogle2FAModal = openBatchChangeGoogle2FAModal;
window.closeBatchChangeGoogle2FAModal = closeBatchChangeGoogle2FAModal;
window.runBatchChangeGoogle2FA = runBatchChangeGoogle2FA;
window.exportUpdatedGoogleAccounts = exportUpdatedGoogleAccounts;
window.loadManageKiroCredentials = loadManageKiroCredentials;
window.onChangeKiroFormat = onChangeKiroFormat;
window.extractCurrentGoogleKiroCreds = extractCurrentGoogleKiroCreds;
window.copyCurrentKiroJSON = copyCurrentKiroJSON;
window.downloadCurrentKiroJSON = downloadCurrentKiroJSON;
window.exportGoogleKiroCredentialsBatch = exportGoogleKiroCredentialsBatch;


