'use strict';
//
// asledger — прогрессивное улучшение web-интерфейса.
//
// Файл намеренно без внешних зависимостей и без сборщика: помощник ввода
// счёта, проверка матрицы ASL-2.2, баланс формы и черновик в браузере.
// Серверная проверка остаётся главной: скрипт лишь помогает не отправлять
// заведомо неверную комбинацию.
(function () {
  // === Денежные величины: целые копейки, без плавающей точки ===

  // parseKop разбирает «1 234,56 ₽» в целые копейки; null при ошибке.
  function parseKop(raw) {
    var s = String(raw)
      .replace(/[\s\u00a0]/g, '')
      .replace(/₽/g, '')
      .replace(/руб\.?/g, '')
      .replace(',', '.');
    if (s === '') {
      return null;
    }
    var negative = s.charAt(0) === '-';
    if (negative) {
      s = s.slice(1);
    }
    var parts = s.split('.');
    if (parts.length > 2) {
      return null;
    }
    var whole = parts[0] === '' ? '0' : parts[0];
    var frac = parts.length === 2 ? parts[1] : '0';
    if (!/^\d+$/.test(whole) || !/^\d+$/.test(frac) || frac.length > 2) {
      return null;
    }
    frac = (frac + '00').slice(0, 2);
    var kop = Number(whole) * 100 + Number(frac);
    return negative ? -kop : kop;
  }

  // formatKop выводит копейки как «1 234,56 ₽».
  function formatKop(kop) {
    var negative = kop < 0;
    kop = negative ? -kop : kop;
    var rub = Math.floor(kop / 100);
    var rest = kop % 100;
    var rubText = String(rub).replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
    return (negative ? '-' : '') + rubText + ',' + (rest < 10 ? '0' + rest : rest) + ' ₽';
  }

  // === Конфигурация операций ===

  function readOps() {
    var node = document.getElementById('entry-config');
    if (!node) {
      return [];
    }
    try {
      return (JSON.parse(node.getAttribute('data-ops')) || {}).ops || [];
    } catch (err) {
      return [];
    }
  }

  function currentOp(ops, form) {
    var checked = form.querySelector('input[name="op_type"]:checked');
    var value = checked ? checked.value : '';
    for (var i = 0; i < ops.length; i++) {
      if (ops[i].type === value) {
        return ops[i];
      }
    }
    return ops[0] || { type: value, roleA: '', roleB: '', labelA: 'Счёт', labelB: 'Счёт' };
  }

  // === Форма ввода проводки ===

  function initEntryForm() {
    var form = document.getElementById('entry-form');
    var ops = readOps();
    var rowsA = document.getElementById('rows-a');
    var rowsB = document.getElementById('rows-b');
    var summary = document.getElementById('balance-summary');
    var submit = document.getElementById('submit');

    if (!form || !rowsA || !rowsB || !summary || !submit) {
      return;
    }

    var containers = [rowsA, rowsB];

    // blankRow хранит пустую строку для клонирования.
    var blankRows = {
      a: rowsA.querySelector('[data-row]'),
      b: rowsB.querySelector('[data-row]')
    };

    function anyError() {
      var error = false;
      containers.forEach(function (container) {
        container.querySelectorAll('[data-row]').forEach(function (row) {
          if (row.dataset.valid === '0') {
            error = true;
          }
        });
      });
      return error;
    }

    // rowState описывает состояние одной строки формы.
    function rowState(row) {
      var hidden = row.querySelector('input[type="hidden"]');
      var amount = row.querySelector('input[name$="_amount"]');
      var id = hidden ? hidden.value.trim() : '';
      var raw = amount ? amount.value.trim() : '';
      var kop = raw === '' ? null : parseKop(raw);
      return {
        row: row,
        amount: amount,
        empty: id === '' && raw === '',
        complete: id !== '' && raw !== '',
        kop: kop,
        invalid: row.dataset.valid === '0'
      };
    }

    function sumSide(container) {
      var total = 0;
      container.querySelectorAll('[data-row]').forEach(function (row) {
        var state = rowState(row);
        if (state.complete && state.kop !== null) {
          total += state.kop;
        }
      });
      return total;
    }

    // recompute проверяет баланс Σ(статьи) = Σ(источники) и готовность формы.
    function recompute() {
      var totalA = sumSide(rowsA);
      var totalB = sumSide(rowsB);
      var incomplete = 0;
      containers.forEach(function (container) {
        container.querySelectorAll('[data-row]').forEach(function (row) {
          var state = rowState(row);
          if (!state.empty && !state.complete) {
            incomplete++;
          }
          if (state.kop === null && state.amount && state.amount.value.trim() !== '') {
            row.dataset.valid = '0';
            setWarning(row, 'сумма должна быть числом, например «1 234,56»');
          }
        });
      });

      var blocked = incomplete > 0 || anyError();
      var message;

      if (totalA === 0 && totalB === 0) {
        message = 'Заполните хотя бы по одной строке с каждой стороны.';
      } else if (totalA === totalB) {
        message = 'Суммы совпадают: ' + formatKop(totalA) + '.';
      } else if (totalA > totalB) {
        message = 'Не хватает источников на ' + formatKop(totalA - totalB) + ': добавьте строку, а не урезайте сумму.';
      } else {
        message = 'Источники больше сумм на ' + formatKop(totalB - totalA) + ': проверьте строки.';
      }

      if (incomplete > 0) {
        message = 'Заполните счёт и сумму во всех начатых строках.';
      }
      if (anyError()) {
        message = 'Есть недопустимые счёта для этой операции — исправьте подсвеченные строки.';
      }

      summary.textContent = message;
      summary.classList.toggle('bad', blocked || totalA !== totalB || totalA === 0);
      submit.disabled = blocked || totalA !== totalB || totalA === 0;
      saveDraft();
      return !submit.disabled;
    }

    function setWarning(row, text) {
      var warning = row.querySelector('.matrix-warning');
      if (!warning) {
        return;
      }
      if (text) {
        warning.textContent = text;
        warning.hidden = false;
      } else {
        warning.textContent = '';
        warning.hidden = true;
      }
    }

    // checkMatrix проверяет допустимость счёта по матрице ASL-2.2.
    function checkMatrix(row, container) {
      var hidden = row.querySelector('input[type="hidden"]');
      var id = hidden ? hidden.value.trim() : '';
      if (id === '') {
        row.dataset.valid = '1';
        setWarning(row, '');
        return Promise.resolve();
      }
      var op = currentOp(ops, form);
      var role = container.dataset.role || '';
      var url = '/matrix.json?op_type=' + encodeURIComponent(op.type) +
        '&role=' + encodeURIComponent(role) +
        '&account_id=' + encodeURIComponent(id);
      return fetch(url, { headers: { Accept: 'application/json' } })
        .then(function (resp) { return resp.json(); })
        .then(function (data) {
          if (data && data.allowed) {
            row.dataset.valid = '1';
            setWarning(row, '');
          } else {
            row.dataset.valid = '0';
            setWarning(row, (data && data.message) || 'недопустимый счёт для этой операции');
          }
        })
        .catch(function () {
          row.dataset.valid = '1';
        });
    }

    function refreshMatrix() {
      containers.forEach(function (container) {
        container.querySelectorAll('[data-row]').forEach(function (row) {
          checkMatrix(row, container).then(recompute);
        });
      });
    }

    // === Автодополнение счёта ===

    function loadSuggestions(container, query) {
      var op = currentOp(ops, form);
      var role = container.dataset.role || '';
      var url = '/suggest.json?q=' + encodeURIComponent(query) +
        '&op_type=' + encodeURIComponent(op.type) +
        '&role=' + encodeURIComponent(role) + '&limit=12';
      return fetch(url, { headers: { Accept: 'application/json' } })
        .then(function (resp) { return resp.json(); })
        .then(function (data) { return (data && data.items) || []; })
        .catch(function () { return []; });
    }

    function renderList(list, items, onPick) {
      list.textContent = '';
      if (!items.length) {
        list.hidden = true;
        return;
      }
      items.forEach(function (item) {
        var li = document.createElement('li');
        li.setAttribute('role', 'option');
        li.setAttribute('aria-selected', 'false');
        if (!item.usable) {
          li.classList.add('disabled');
        }
        var path = document.createElement('span');
        path.className = 'ac-path';
        path.textContent = item.path;
        li.appendChild(path);

        var note = document.createElement('span');
        note.className = 'ac-note';
        if (!item.is_leaf) {
          note.textContent = 'папка';
        } else if (!item.usable) {
          note.textContent = item.reserved ? 'зарезервировано (S5)' : 'недопустимый тип';
        } else {
          note.textContent = item.type + (item.cell ? ' · ' + item.cell : '');
        }
        li.appendChild(note);

        li.addEventListener('mousedown', function (event) {
          event.preventDefault();
          onPick(item);
        });
        list.appendChild(li);
      });
      list.hidden = false;
    }

    function initRow(row, container) {
      if (row.dataset.init === '1') {
        return;
      }
      row.dataset.init = '1';

      var input = row.querySelector('.ac-input');
      var hidden = row.querySelector('input[type="hidden"]');
      var list = row.querySelector('.ac-list');
      var timer = null;
      var items = [];

      function close() {
        list.hidden = true;
        input.setAttribute('aria-expanded', 'false');
      }

      function pick(item) {
        if (!item.is_leaf) {
          // Папка: подставляем первый листовой потомок, иначе раскрываем список.
          loadSuggestions(container, item.path + ':').then(function (children) {
            var leaf = null;
            for (var i = 0; i < children.length; i++) {
              if (children[i].is_leaf && children[i].usable) {
                leaf = children[i];
                break;
              }
            }
            if (leaf) {
              applyLeaf(leaf);
            } else {
              items = children;
              renderList(list, items, pick);
              input.setAttribute('aria-expanded', 'true');
            }
          });
          return;
        }
        applyLeaf(item);
      }

      function applyLeaf(item) {
        input.value = item.path;
        hidden.value = item.id;
        close();
        checkMatrix(row, container).then(recompute);
      }

      function search() {
        var query = input.value.trim();
        if (query === '') {
          hidden.value = '';
          close();
          checkMatrix(row, container).then(recompute);
          return;
        }
        loadSuggestions(container, query).then(function (found) {
          items = found;
          renderList(list, items, pick);
          input.setAttribute('aria-expanded', 'true');
        });
      }

      input.addEventListener('input', function () {
        hidden.value = '';
        row.dataset.valid = '1';
        setWarning(row, '');
        if (timer) {
          clearTimeout(timer);
        }
        timer = setTimeout(search, 140);
      });

      input.addEventListener('focus', function () {
        if (input.value.trim() !== '' && list.hidden) {
          search();
        }
      });

      input.addEventListener('blur', function () {
        setTimeout(close, 150);
      });

      input.addEventListener('keydown', function (event) {
        if (list.hidden) {
          return;
        }
        var options = Array.prototype.slice.call(list.querySelectorAll('li'));
        var current = options.findIndex(function (li) { return li.getAttribute('aria-selected') === 'true'; });
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
          event.preventDefault();
          var next = event.key === 'ArrowDown' ? current + 1 : current - 1;
          if (next < 0) {
            next = 0;
          }
          if (next >= options.length) {
            next = options.length - 1;
          }
          options.forEach(function (li, idx) {
            li.setAttribute('aria-selected', idx === next ? 'true' : 'false');
          });
        } else if (event.key === 'Enter') {
          if (current >= 0 && items[current]) {
            event.preventDefault();
            pick(items[current]);
          }
        } else if (event.key === 'Escape') {
          close();
        }
      });

      if (hidden.value !== '') {
        checkMatrix(row, container).then(recompute);
      }
    }

    // addRow добавляет пустую строку в секцию.
    function addRow(container, values) {
      var blank = blankRows[container.id === 'rows-a' ? 'a' : 'b'];
      var row = blank.cloneNode(true);
      row.dataset.init = '0';
      row.dataset.valid = '1';
      var input = row.querySelector('.ac-input');
      var hidden = row.querySelector('input[type="hidden"]');
      var amount = row.querySelector('input[name$="_amount"]');
      input.value = values ? values.account || '' : '';
      hidden.value = values ? values.id || '' : '';
      amount.value = values ? values.amount || '' : '';
      row.querySelector('.ac-list').hidden = true;
      row.querySelector('.matrix-warning').hidden = true;
      container.appendChild(row);
      initRow(row, container);
      return row;
    }

    containers.forEach(function (container) {
      container.querySelectorAll('[data-row]').forEach(function (row) { initRow(row, container); });
    });

    containers.forEach(function (container) {
      var addButton = document.querySelector('[data-add="' + container.id + '"]');
      if (addButton) {
        addButton.addEventListener('click', function () { addRow(container); recompute(); });
      }
      container.addEventListener('click', function (event) {
        if (!event.target.classList.contains('remove')) {
          return;
        }
        var rows = container.querySelectorAll('[data-row]');
        if (rows.length > 1) {
          event.target.closest('[data-row]').remove();
          recompute();
        }
      });
      container.addEventListener('input', function (event) {
        if (event.target.name && /_amount$/.test(event.target.name)) {
          recompute();
        }
      });
    });

    form.querySelectorAll('input[name="op_type"]').forEach(function (radio) {
      radio.addEventListener('change', function () {
        var op = currentOp(ops, form);
        document.getElementById('section-a-title').textContent = op.labelA;
        document.getElementById('section-b-title').textContent = op.labelB;
        rowsA.dataset.role = op.roleA;
        rowsB.dataset.role = op.roleB;
        refreshMatrix();
      });
    });

    // === Черновик формы в браузере ===

    var draftKey = 'asledger.entry.draft';

    function saveDraft() {
      try {
        var draft = {
          op_type: currentOp(ops, form).type,
          date: form.querySelector('#date').value,
          description: form.querySelector('#description').value,
          actor_id: form.querySelector('#actor_id').value,
          a: [], b: []
        };
        containers.forEach(function (container, index) {
          container.querySelectorAll('[data-row]').forEach(function (row) {
            var hidden = row.querySelector('input[type="hidden"]');
            var amount = row.querySelector('input[name$="_amount"]');
            draft[index === 0 ? 'a' : 'b'].push({
              id: hidden.value,
              account: row.querySelector('.ac-input').value,
              amount: amount.value
            });
          });
        });
        localStorage.setItem(draftKey, JSON.stringify(draft));
        var note = document.getElementById('draft-note');
        if (note) {
          note.hidden = false;
        }
      } catch (err) {
        /* приватный режим браузера — черновик просто не сохраняется */
      }
    }

    function restoreDraft() {
      var raw;
      try {
        raw = localStorage.getItem(draftKey);
      } catch (err) {
        return false;
      }
      if (!raw) {
        return false;
      }
      var draft;
      try {
        draft = JSON.parse(raw);
      } catch (err) {
        return false;
      }
      var hasPrefill = form.querySelectorAll('input[type="hidden"]');
      var filled = Array.prototype.some.call(hasPrefill, function (node) {
        return node.value !== '';
      });
      if (filled || form.querySelector('.flash-error')) {
        return false;
      }
      var radio = form.querySelector('input[name="op_type"][value="' + draft.op_type + '"]');
      if (radio) {
        radio.checked = true;
        var op = currentOp(ops, form);
        document.getElementById('section-a-title').textContent = op.labelA;
        document.getElementById('section-b-title').textContent = op.labelB;
        rowsA.dataset.role = op.roleA;
        rowsB.dataset.role = op.roleB;
      }
      if (draft.date) {
        form.querySelector('#date').value = draft.date;
      }
      if (draft.description) {
        form.querySelector('#description').value = draft.description;
      }
      if (draft.actor_id) {
        form.querySelector('#actor_id').value = draft.actor_id;
      }
      fillSection(rowsA, 'a', draft.a);
      fillSection(rowsB, 'b', draft.b);
      return true;
    }

    function fillSection(container, key, rows) {
      if (!rows || !rows.length) {
        return;
      }
      var existing = container.querySelectorAll('[data-row]');
      existing.forEach(function (row, index) {
        if (index > 0) {
          row.remove();
        }
      });
      var first = container.querySelector('[data-row]');
      var input = first.querySelector('.ac-input');
      var hidden = first.querySelector('input[type="hidden"]');
      var amount = first.querySelector('input[name$="_amount"]');
      input.value = rows[0].account || '';
      hidden.value = rows[0].id || '';
      amount.value = rows[0].amount || '';
      for (var i = 1; i < rows.length; i++) {
        addRow(container, rows[i]);
      }
    }

    var clearButton = document.getElementById('clear-draft');
    if (clearButton) {
      clearButton.addEventListener('click', function () {
        try {
          localStorage.removeItem(draftKey);
        } catch (err) {
          /* игнорируем */
        }
        containers.forEach(function (container) {
          var rows = container.querySelectorAll('[data-row]');
          rows.forEach(function (row, index) {
            if (index > 0) {
              row.remove();
            } else {
              row.querySelector('.ac-input').value = '';
              row.querySelector('input[type="hidden"]').value = '';
              row.querySelector('input[name$="_amount"]').value = '';
              setWarning(row, '');
              row.dataset.valid = '1';
            }
          });
        });
        recompute();
      });
    }

    form.addEventListener('submit', function (event) {
      if (!recompute()) {
        event.preventDefault();
        return;
      }
      try {
        localStorage.removeItem(draftKey);
      } catch (err) {
        /* игнорируем */
      }
    });

    restoreDraft();
    refreshMatrix();
    recompute();
  }

  // === Начальные остатки: строка капитала ===

  function initInitialForm() {
    var form = document.getElementById('initial-form');
    var capital = document.getElementById('capital-line');
    var submit = document.getElementById('initial-submit');
    if (!form || !capital) {
      return;
    }

    function recompute() {
      var sumAssets = 0;
      var sumLiabilities = 0;
      var filled = 0;
      form.querySelectorAll('[data-side]').forEach(function (row) {
        var input = row.querySelector('[data-amount]');
        var raw = input.value.trim();
        if (raw === '') {
          return;
        }
        var kop = parseKop(raw);
        if (kop === null) {
          row.dataset.valid = '0';
          return;
        }
        row.dataset.valid = '1';
        filled++;
        if (row.dataset.side === 'asset') {
          sumAssets += kop;
        } else {
          sumLiabilities += kop;
        }
      });

      var diff = sumAssets - sumLiabilities;
      var hint = diff < 0
        ? 'обязательства больше активов на ' + formatKop(-diff) + ' (капитал с отрицательным знаком)'
        : 'капитал ' + formatKop(diff);
      capital.textContent = 'Строка капитала: ' + hint;
      capital.classList.toggle('bad', filled === 0);

      var invalid = form.querySelector('[data-side][data-valid="0"]');
      if (submit) {
        submit.disabled = filled === 0 || invalid !== null;
      }
    }

    form.addEventListener('input', recompute);
    recompute();
  }

  // === Списки покупок ===

  // initShoppingForm добавляет товары без перезагрузки страницы и подсказывает
  // статьи расходов по плану счетов и истории трат.
  function initShoppingForm() {
    var form = document.getElementById('shopping-add-form');
    var list = document.getElementById('shopping-items');
    if (!form || !list) {
      return;
    }
    var errorBox = document.getElementById('shopping-add-error');
    var accountInput = document.getElementById('add-account');
    var accountHidden = document.getElementById('add-account-id');
    var accountList = document.getElementById('add-account-list');
    var suggestURL = form.getAttribute('data-suggest') || '/shopping/suggest.json';
    var items = [];
    var timer = null;

    function closeList() {
      if (!accountList) {
        return;
      }
      accountList.hidden = true;
      accountInput.setAttribute('aria-expanded', 'false');
    }

    function pick(item) {
      accountInput.value = item.path;
      accountHidden.value = String(item.id);
      closeList();
    }

    function renderList(found) {
      items = found;
      accountList.textContent = '';
      if (!found.length) {
        closeList();
        return;
      }
      found.forEach(function (item) {
        var li = document.createElement('li');
        li.setAttribute('role', 'option');
        li.setAttribute('aria-selected', 'false');
        var path = document.createElement('span');
        path.className = 'ac-path';
        path.textContent = item.path;
        li.appendChild(path);
        if (item.used > 0) {
          var note = document.createElement('span');
          note.className = 'ac-note';
          note.textContent = 'использований: ' + item.used;
          li.appendChild(note);
        }
        li.addEventListener('mousedown', function (event) {
          event.preventDefault();
          pick(item);
        });
        accountList.appendChild(li);
      });
      accountList.hidden = false;
      accountInput.setAttribute('aria-expanded', 'true');
    }

    function search() {
      fetch(suggestURL + '?q=' + encodeURIComponent(accountInput.value.trim()) + '&limit=12', {
        headers: { Accept: 'application/json' }
      })
        .then(function (resp) { return resp.json(); })
        .then(function (data) { renderList((data && data.items) || []); })
        .catch(closeList);
    }

    if (accountInput && accountHidden && accountList) {
      accountInput.addEventListener('input', function () {
        accountHidden.value = '';
        if (timer) {
          clearTimeout(timer);
        }
        timer = setTimeout(search, 140);
      });
      accountInput.addEventListener('focus', function () {
        if (accountInput.value.trim() !== '') {
          search();
        }
      });
      accountInput.addEventListener('blur', function () { setTimeout(closeList, 150); });
      accountInput.addEventListener('keydown', function (event) {
        if (accountList.hidden) {
          return;
        }
        var options = Array.prototype.slice.call(accountList.querySelectorAll('li'));
        var current = options.findIndex(function (li) {
          return li.getAttribute('aria-selected') === 'true';
        });
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
          event.preventDefault();
          var next = event.key === 'ArrowDown' ? current + 1 : current - 1;
          if (next < 0) {
            next = 0;
          }
          if (next >= options.length) {
            next = options.length - 1;
          }
          options.forEach(function (li, idx) {
            li.setAttribute('aria-selected', idx === next ? 'true' : 'false');
          });
        } else if (event.key === 'Enter') {
          if (current >= 0 && items[current]) {
            event.preventDefault();
            pick(items[current]);
          }
        } else if (event.key === 'Escape') {
          closeList();
        }
      });
    }

    function showError(message) {
      if (!errorBox) {
        return;
      }
      errorBox.textContent = message || '';
      errorBox.hidden = !message;
    }

    function setText(id, value) {
      var node = document.getElementById(id);
      if (node && value !== undefined && value !== null) {
        node.textContent = value;
      }
    }

    function updateSummary(data) {
      setText('shopping-item-count', data.item_count);
      setText('shopping-confirmed-count', data.confirmed_count);
      setText('shopping-confirmed-total', data.confirmed_total_kop);
      var empty = document.getElementById('shopping-empty');
      if (empty && data.item_count > 0) {
        empty.hidden = true;
      }
    }

    form.addEventListener('submit', function (event) {
      if (!window.fetch || !window.FormData) {
        return;
      }
      event.preventDefault();
      showError('');
      var button = form.querySelector('button[type="submit"]');
      if (button) {
        button.disabled = true;
      }
      fetch(form.action, {
        method: 'POST',
        headers: { Accept: 'application/json' },
        body: new URLSearchParams(new FormData(form))
      })
        .then(function (resp) {
          return resp.json().then(function (data) {
            return { ok: resp.ok, data: data };
          });
        })
        .then(function (result) {
          if (!result.ok) {
            showError((result.data && result.data.error) || 'не удалось добавить позицию');
            return;
          }
          list.insertAdjacentHTML('beforeend', result.data.html);
          form.reset();
          accountHidden.value = '';
          updateSummary(result.data);
          var name = document.getElementById('add-name');
          if (name) {
            name.focus();
          }
        })
        .catch(function () {
          // Сеть недоступна — отправляем обычной формой.
          form.submit();
        })
        .finally(function () {
          if (button) {
            button.disabled = false;
          }
        });
    });
  }

  // === Продукты: ставки, суммы, ступени и обязательный предпросмотр ===

  // parseRatePPM разбирает проценты в ppm; null при ошибке.
  function parseRatePPM(raw) {
    var s = String(raw)
      .replace(/[\s\u00a0]/g, '')
      .replace(/%/g, '')
      .replace(',', '.');
    if (s === '') {
      return null;
    }
    var parts = s.split('.');
    if (parts.length > 2) {
      return null;
    }
    var whole = parts[0] === '' ? '0' : parts[0];
    var frac = parts.length === 2 ? parts[1] : '';
    if (!/^\d+$/.test(whole) || (frac !== '' && !/^\d+$/.test(frac)) || frac.length > 4) {
      return null;
    }
    while (frac.length < 4) {
      frac += '0';
    }
    var ppm = Number(whole) * 10000 + Number(frac);
    if (ppm > 1000000) {
      return null;
    }
    return ppm;
  }

  // initProductForm проверяет форму настроек до отправки на сервер.
  function initProductForm() {
    var form = document.querySelector('[data-product-form]');
    if (!form) {
      return;
    }
    var errorBox = document.querySelector('.form-error');
    var steps = form.querySelector('[data-steps]');
    var template = document.getElementById('step-template');

    function mark(input, bad) {
      input.classList.toggle('invalid', bad);
      input.setAttribute('aria-invalid', bad ? 'true' : 'false');
    }

    function showError(message) {
      if (!errorBox) {
        return;
      }
      errorBox.textContent = message || '';
      errorBox.hidden = !message;
    }

    function validateMoney(input, required) {
      var raw = input.value.trim();
      if (raw === '') {
        mark(input, required);
        return required ? 'Укажите сумму, например 1 000,00.' : '';
      }
      if (parseKop(raw) === null) {
        mark(input, true);
        return 'Сумма должна быть числом, например 1 000,00.';
      }
      mark(input, false);
      return '';
    }

    function validateRate(input, required) {
      var raw = input.value.trim();
      if (raw === '') {
        mark(input, required);
        return required ? 'Укажите ставку в процентах, например 15,5.' : '';
      }
      if (parseRatePPM(raw) === null) {
        mark(input, true);
        return 'Ставка должна быть числом от 0 до 100, например 15,5.';
      }
      mark(input, false);
      return '';
    }

    function validateSteps() {
      if (!steps) {
        return '';
      }
      var rows = steps.querySelectorAll('.step-row');
      for (var i = 0; i < rows.length; i++) {
        var row = rows[i];
        var from = row.querySelector('[data-step-from]');
        var to = row.querySelector('[data-step-to]');
        var rate = row.querySelector('[data-step-rate]');
        var message = validateMoney(from, true) || validateRate(rate, true);
        if (message) {
          return 'Ступень ' + (i + 1) + ': ' + message;
        }
        var fromKop = parseKop(from.value);
        if (to && to.value.trim() !== '') {
          var toKop = parseKop(to.value);
          if (toKop === null) {
            mark(to, true);
            return 'Ступень ' + (i + 1) + ': верхняя граница должна быть числом.';
          }
          mark(to, false);
          if (toKop <= fromKop) {
            mark(to, true);
            return 'Ступень ' + (i + 1) + ': верхняя граница должна быть больше нижней.';
          }
        } else if (to) {
          mark(to, false);
        }
      }
      return '';
    }

    function validate() {
      var inputs = form.querySelectorAll('[data-rate]');
      for (var i = 0; i < inputs.length; i++) {
        var message = validateRate(inputs[i], inputs[i].hasAttribute('data-required'));
        if (message) {
          showError(message);
          inputs[i].focus();
          return false;
        }
      }
      inputs = form.querySelectorAll('[data-money]');
      for (var j = 0; j < inputs.length; j++) {
        message = validateMoney(inputs[j], inputs[j].hasAttribute('data-required'));
        if (message) {
          showError(message);
          inputs[j].focus();
          return false;
        }
      }
      message = validateSteps();
      if (message) {
        showError(message);
        return false;
      }
      showError('');
      return true;
    }

    form.addEventListener('submit', function (event) {
      if (!validate()) {
        event.preventDefault();
      }
    });

    if (steps) {
      steps.addEventListener('click', function (event) {
        if (!event.target.classList.contains('step-remove')) {
          return;
        }
        event.target.closest('.step-row').remove();
      });
    }

    var addStep = form.querySelector('[data-add-step]');
    if (addStep && steps && template) {
      addStep.addEventListener('click', function () {
        var fragment = template.content.cloneNode(true);
        steps.appendChild(fragment);
        var last = steps.querySelector('.step-row:last-child [data-step-from]');
        if (last) {
          last.focus();
        }
      });
    }
  }

  // initPreviewGuards не даёт провести операцию без предпросмотра.
  function initPreviewGuards() {
    document.querySelectorAll('[data-requires-preview]').forEach(function (button) {
      var target = document.getElementById(button.getAttribute('data-requires-preview'));
      if (!target) {
        button.disabled = true;
        button.title = 'Сначала нажмите «Рассчитать» и проверьте разбивку.';
      }
    });
  }

  document.addEventListener('DOMContentLoaded', function () {
    if (document.getElementById('entry-form')) {
      initEntryForm();
    }
    if (document.getElementById('initial-form')) {
      initInitialForm();
    }
    if (document.getElementById('shopping-add-form')) {
      initShoppingForm();
    }
    if (document.querySelector('[data-product-form]')) {
      initProductForm();
    }
    initPreviewGuards();
  });
})();
