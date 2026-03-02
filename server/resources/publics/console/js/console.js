// Mdbrain Frontend Helpers

function escapeHtmlAttr(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#x27;');
}

function normalizeClassName(value) {
  return String(value || '')
    .split(/\s+/)
    .map((s) => s.trim())
    .filter(Boolean)
    .filter((s, i, a) => a.indexOf(s) === i)
    .join(' ');
}

function lucideIconSvg(name, className = '', ariaLabel = '') {
  const icons = {
    // Aliases / legacy names used in templates
    'check-circle': `<path d="M21.801 10A10 10 0 1 1 17 3.335" /><path d="m9 11 3 3L22 4" />`,
    'alert-circle': `<circle cx="12" cy="12" r="10" /><line x1="12" x2="12" y1="8" y2="12" /><line x1="12" x2="12.01" y1="16" y2="16" />`,
    info: `<circle cx="12" cy="12" r="10" /><path d="M12 16v-4" /><path d="M12 8h.01" />`,
    'alert-triangle': `<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3" /><path d="M12 9v4" /><path d="M12 17h.01" />`,

    // Direct icon names used by JS logic
    check: `<path d="M20 6 9 17l-5-5" />`,
    eye: `<path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0" /><circle cx="12" cy="12" r="3" />`,
    'eye-off': `<path d="M10.733 5.076a10.744 10.744 0 0 1 11.205 6.575 1 1 0 0 1 0 .696 10.747 10.747 0 0 1-1.444 2.49" /><path d="M14.084 14.158a3 3 0 0 1-4.242-4.242" /><path d="M17.479 17.499a10.75 10.75 0 0 1-15.417-5.151 1 1 0 0 1 0-.696 10.75 10.75 0 0 1 4.446-5.143" /><path d="m2 2 20 20" />`
  };

  const body = icons[name];
  if (!body) return '';

  const classes = normalizeClassName(`lucide lucide-${name} ${className}`);
  const label = String(ariaLabel || '').trim();
  const a11yAttrs = label
    ? ` role="img" aria-label="${escapeHtmlAttr(label)}"`
    : ' aria-hidden="true" focusable="false"';

  return `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="${escapeHtmlAttr(classes)}" data-lucide="${escapeHtmlAttr(name)}"${a11yAttrs}>${body}</svg>`;
}

/**
 * Copy text to clipboard with notification
 * @param {string} text - Text to copy
 */
function copyToClipboard(text) {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(text).then(() => {
      showNotification('Copied to clipboard', 'success');
    }).catch(err => {
      console.error('Failed to copy:', err);
      showNotification('Copy failed', 'error');
    });
  } else {
    // Fallback for older browsers
    const textArea = document.createElement('textarea');
    textArea.value = text;
    textArea.style.position = 'fixed';
    textArea.style.left = '-999999px';
    document.body.appendChild(textArea);
    textArea.select();
    try {
      document.execCommand('copy');
      showNotification('Copied to clipboard', 'success');
    } catch (err) {
      console.error('Failed to copy:', err);
      showNotification('Copy failed', 'error');
    }
    document.body.removeChild(textArea);
  }
}

function showNotification(message, type = 'info', duration = 3000) {
  const container = document.getElementById('notification-container');
  if (!container) return;

  const icons = {
    success: 'check-circle',
    error: 'alert-circle',
    info: 'info',
    warning: 'alert-triangle'
  };

  const notification = document.createElement('div');
  notification.className = `toast toast-${type}`;
  notification.innerHTML = `
    ${lucideIconSvg(icons[type] || icons.info, 'icon-sm')}
    <span>${message}</span>
  `;

  container.appendChild(notification);

  setTimeout(() => {
    notification.classList.add('toast-out');
    setTimeout(() => notification.remove(), 200);
  }, duration);
}

/**
 * Format date string to locale format
 * @param {string} dateStr - Date string to format
 * @returns {string} Formatted date string
 */
function formatDate(dateStr) {
  if (!dateStr) return '';

  const date = new Date(dateStr);
  return date.toLocaleString(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  });
}

function renderLocalDatetimes(root = document) {
  const scope = root && root.querySelectorAll ? root : document;
  scope.querySelectorAll('[data-utc-datetime]').forEach((el) => {
    const raw = el.getAttribute('data-utc-datetime');
    if (!raw) return;

    const normalized = /[zZ]$|[+-]\d{2}:?\d{2}$/.test(raw)
      ? raw
      : /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(raw)
        ? `${raw.replace(' ', 'T')}Z`
        : raw;

    const date = new Date(normalized);
    if (Number.isNaN(date.getTime())) return;
    el.textContent = date.toLocaleString(undefined, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    });
    el.setAttribute('title', raw);
  });
}

function safeParseJson(text) {
  if (!text || typeof text !== 'string') return null;
  try {
    return JSON.parse(text);
  } catch (e) {
    return null;
  }
}

function getHtmxJsonResponse(event) {
  const xhr = event?.detail?.xhr;
  if (!xhr) return null;
  return safeParseJson(xhr.responseText || xhr.response || '') || null;
}

function renderAuthMessage(target, type, message) {
  if (!target) return;

  const isSuccess = type === 'success';
  const icon = isSuccess ? 'check-circle' : 'alert-circle';
  const cls = isSuccess ? 'alert-success' : 'alert-error';

  target.innerHTML = `
    <div class="alert ${cls}">
      ${lucideIconSvg(icon, 'icon-sm')}
      <span>${escapeHtmlAttr(message)}</span>
    </div>
  `;
}

document.addEventListener('DOMContentLoaded', () => {
  console.log('Mdbrain console initialized');
  renderLocalDatetimes(document);
});

document.addEventListener('htmx:afterSwap', (event) => {
  const target = (event.detail && event.detail.target) || event.target || document;
  renderLocalDatetimes(target);
});

document.addEventListener('htmx:afterRequest', (event) => {
  const requestPath = event?.detail?.pathInfo?.requestPath;
  if (requestPath !== '/console/login' && requestPath !== '/console/init') return;

  const target = event?.detail?.target;

  if (!event.detail.successful) {
    renderAuthMessage(target, 'error', 'Network error, please try again');
    return;
  }

  const response = getHtmxJsonResponse(event);
  if (!response) {
    renderAuthMessage(target, 'error', 'Server response error');
    return;
  }

  if (response.success) {
    if (requestPath === '/console/login') {
      renderAuthMessage(target, 'success', 'Login successful, redirecting...');
      setTimeout(() => { window.location.href = '/console'; }, 800);
      return;
    }

    renderAuthMessage(target, 'success', 'Account created successfully, redirecting...');
    setTimeout(() => { window.location.href = '/console/login'; }, 1200);
    return;
  }

  const fallbackMessage = requestPath === '/console/login' ? 'Login failed' : 'Creation failed';
  renderAuthMessage(target, 'error', response.error || fallbackMessage);
});

/**
 * Open create modal
 */
function openCreateModal() {
  const modal = document.getElementById('modal-create');
  if (modal) {
    modal.showModal();
  }
}

/**
 * Close create modal
 */
function closeCreateModal() {
  const modal = document.getElementById('modal-create');
  if (modal) {
    modal.classList.add('closing');
    modal.addEventListener('animationend', () => {
      modal.close();
      modal.classList.remove('closing');
    }, { once: true });
  }
}

/**
 * Open edit modal with vault data
 * @param {string} id - Vault ID
 * @param {string} name - Vault name
 * @param {string} domain - Vault domain
 */
function openEditModal(id, name, domain) {
  const modal = document.getElementById('modal-edit');
  if (modal) {
    // Fill form fields
    const vaultIdInput = document.getElementById('edit-vault-id');
    const nameInput = document.getElementById('edit-name');
    const domainInput = document.getElementById('edit-domain');

    if (vaultIdInput) vaultIdInput.value = id;
    if (nameInput) nameInput.value = name;
    if (domainInput) domainInput.value = domain;

    // Update form action URL
    const form = document.getElementById('edit-form');
    if (form) {
      form.setAttribute('hx-put', `/console/vaults/${id}`);
      // Re-process HTMX attributes after dynamic update
      htmx.process(form);
    }

    modal.showModal();
  }
}

/**
 * Close edit modal
 */
function closeEditModal() {
  const modal = document.getElementById('modal-edit');
  if (modal) {
    modal.classList.add('closing');
    modal.addEventListener('animationend', () => {
      modal.close();
      modal.classList.remove('closing');
    }, { once: true });
  }
}

// ============================================================
// Change Password Modal
// ============================================================

function openChangePasswordModal() {
  const modal = document.getElementById('modal-change-password');
  const form = document.getElementById('change-password-form');
  if (form) form.reset();
  if (modal) modal.showModal();
}

function closeChangePasswordModal() {
  const modal = document.getElementById('modal-change-password');
  if (modal) {
    modal.classList.add('closing');
    modal.addEventListener('animationend', () => {
      modal.close();
      modal.classList.remove('closing');
    }, { once: true });
  }
}

// Setup backdrop click handlers
document.addEventListener('DOMContentLoaded', () => {
  const createModal = document.getElementById('modal-create');
  const editModal = document.getElementById('modal-edit');
  const changePasswordModal = document.getElementById('modal-change-password');

  // Click outside modal content to close
  [createModal, editModal, changePasswordModal].forEach(modal => {
    if (modal) {
      modal.addEventListener('click', (e) => {
        // Check if clicked on content or its children
        const clickedOnContent = e.target.closest('.modal-content');
        if (!clickedOnContent) {
          // Clicked on backdrop, close modal
          if (modal.id === 'modal-create') {
            closeCreateModal();
          } else if (modal.id === 'modal-edit') {
            closeEditModal();
          } else if (modal.id === 'modal-change-password') {
            closeChangePasswordModal();
          }
        }
      });
    }
  });
});

document.addEventListener('htmx:beforeRequest', function(event) {
  if (event.detail.pathInfo.requestPath !== '/console/user/password') return;

  const form = event.detail.elt;
  const newInput = form && form.querySelector && form.querySelector('[name="new-password"]');
  const confirmInput = form && form.querySelector && form.querySelector('[name="confirm-password"]');
  const newPassword = newInput ? newInput.value : '';
  const confirmPassword = confirmInput ? confirmInput.value : '';

  if (newPassword !== confirmPassword) {
    event.preventDefault();
    showNotification('New password confirmation does not match', 'error');
  }
});

// Expose global functions
window.copyToClipboard = copyToClipboard;
window.showNotification = showNotification;
window.formatDate = formatDate;
window.openCreateModal = openCreateModal;
window.closeCreateModal = closeCreateModal;
window.openEditModal = openEditModal;
window.closeEditModal = closeEditModal;
window.openChangePasswordModal = openChangePasswordModal;
window.closeChangePasswordModal = closeChangePasswordModal;

document.addEventListener('htmx:afterRequest', function(event) {
  if (event.detail.pathInfo.requestPath !== '/console/user/password') return;

  try {
    const response = JSON.parse(event.detail.xhr.response || '{}');
    if (response.success) {
      showNotification('Password updated', 'success');
      closeChangePasswordModal();
    } else {
      showNotification(response.error || 'Password update failed', 'error');
    }
  } catch (e) {
    showNotification('Server response error', 'error');
  }
});

function filterNotes(vaultId, query) {
  const container = document.getElementById(`note-selector-${vaultId}`);
  if (!container) return;
  
  const items = container.querySelectorAll('.note-item');
  const q = query.toLowerCase().trim();
  
  items.forEach(item => {
    const path = item.dataset.path;
    item.style.display = (!q || path.includes(q)) ? '' : 'none';
  });
}

function toggleNoteSelector(vaultId) {
  const container = document.getElementById(`note-selector-${vaultId}`);
  if (!container) return;
  
  const wasOpen = container.classList.contains('open');
  
  document.querySelectorAll('.note-selector.open').forEach(el => el.classList.remove('open'));
  
  if (!wasOpen) {
    container.classList.add('open');
    const input = container.querySelector('.note-search-input');
    if (input) {
      input.value = '';
      filterNotes(vaultId, '');
      setTimeout(() => input.focus(), 10);
    }
  }
}

function handleNoteSelect(vaultId, element, event) {
  const container = document.getElementById(`note-selector-${vaultId}`);
  if (!container) return;

  if (event.detail.successful) {
    container.querySelectorAll('.note-item').forEach(item => {
      item.classList.remove('selected');
      const icon = item.querySelector('.note-check');
      if (icon) icon.remove();
    });
    
    element.classList.add('selected');
    if (!element.querySelector('.note-check')) {
      element.insertAdjacentHTML('beforeend', lucideIconSvg('check', 'icon-xs note-check'));
    }
    
    const trigger = container.querySelector('.note-selector-value');
    if (trigger) {
      trigger.textContent = element.dataset.display;
    }
    
    container.classList.remove('open');
    showNotification('Homepage updated', 'success');
  } else {
    showNotification('Update failed', 'error');
  }
}

document.addEventListener('click', (e) => {
  if (!e.target.closest('.note-selector')) {
    document.querySelectorAll('.note-selector.open').forEach(el => el.classList.remove('open'));
  }
});

window.filterNotes = filterNotes;
window.toggleNoteSelector = toggleNoteSelector;
window.handleNoteSelect = handleNoteSelect;

// ============================================================
// Vault Actions Menu
// ============================================================

function toggleActionMenu(button) {
  const menu = button.nextElementSibling;
  const isOpen = menu.classList.contains('open');
  document.querySelectorAll('.action-menu.open').forEach(m => m.classList.remove('open'));
  if (!isOpen) {
    menu.classList.add('open');
  }
}

document.addEventListener('click', function(e) {
  if (!e.target.closest('.vault-actions')) {
    document.querySelectorAll('.action-menu.open').forEach(m => m.classList.remove('open'));
  }
});

function toggleSyncKey(button, vaultId) {
  const keyElement = document.getElementById(`key-${vaultId}`);
  const fullKey = keyElement.dataset.key;
  const currentText = keyElement.textContent.trim();
  const isHidden = currentText.includes('*');
  const icon = button.querySelector('svg');
  const iconClass = icon?.getAttribute('class') || 'icon-xs';

  if (isHidden) {
    keyElement.textContent = fullKey;
    if (icon) icon.outerHTML = lucideIconSvg('eye-off', iconClass);
    button.title = 'Hide';
  } else {
    const masked = fullKey.substring(0, 8) + '******' + fullKey.substring(fullKey.length - 8);
    keyElement.textContent = masked;
    if (icon) icon.outerHTML = lucideIconSvg('eye', iconClass);
    button.title = 'Show';
  }
}

window.toggleActionMenu = toggleActionMenu;
window.toggleSyncKey = toggleSyncKey;

// ============================================================
// Logo Upload & Delete (HTMX)
// ============================================================

function handleLogoRequest(event) {
  const response = getHtmxJsonResponse(event) || {};
  const verb = event?.detail?.requestConfig?.verb;

  if (event.detail.successful && response.success !== false) {
    showNotification(response.message || (verb === 'delete' ? 'Logo deleted' : 'Logo uploaded'), 'success');
    htmx.trigger('body', 'refreshList');
    return;
  }

  showNotification(response.error || response.message || 'Request failed', 'error');
}

window.handleLogoRequest = handleLogoRequest;

// ============================================================
// Custom HTML Modal
// ============================================================

function openCustomHtmlModal(vaultId) {
  const modal = document.getElementById('modal-custom-html');
  const form = document.getElementById('custom-html-form');
  const vaultIdInput = document.getElementById('custom-html-vault-id');
  const textarea = document.getElementById('custom-html-textarea');
  const dataEl = document.getElementById(`custom-html-data-${vaultId}`);

  if (form) {
    form.setAttribute('hx-put', `/console/vaults/${vaultId}/custom-head-html`);
    htmx.process(form);
  }

  if (modal && vaultIdInput && textarea) {
    vaultIdInput.value = vaultId;
    textarea.value = dataEl ? dataEl.textContent : '';
    modal.showModal();
  }
}

function closeCustomHtmlModal() {
  const modal = document.getElementById('modal-custom-html');
  if (modal) {
    modal.classList.add('closing');
    modal.addEventListener('animationend', () => {
      modal.close();
      modal.classList.remove('closing');
    }, { once: true });
  }
}

function onSaveCustomHtml(event) {
  const response = getHtmxJsonResponse(event) || {};

  if (event.detail.successful && response.success) {
    showNotification('Custom HTML saved', 'success');
    closeCustomHtmlModal();
    htmx.trigger('body', 'refreshList');
    return;
  }

  showNotification(response.error || 'Save failed', 'error');
}

// Setup backdrop click handler for custom HTML modal
document.addEventListener('DOMContentLoaded', () => {
  const customHtmlModal = document.getElementById('modal-custom-html');
  if (customHtmlModal) {
    customHtmlModal.addEventListener('click', (e) => {
      const clickedOnContent = e.target.closest('.modal-content');
      if (!clickedOnContent) {
        closeCustomHtmlModal();
      }
    });
  }
});

window.openCustomHtmlModal = openCustomHtmlModal;
window.closeCustomHtmlModal = closeCustomHtmlModal;
window.onSaveCustomHtml = onSaveCustomHtml;
