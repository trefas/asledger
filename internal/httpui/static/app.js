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

  document.addEventListener('DOMContentLoaded', function () {
    if (document.getElementById('entry-form')) {
      initEntryForm();
    }
  });
})();
