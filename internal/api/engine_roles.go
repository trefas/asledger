package api

import (
	"net/http"
	"strings"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/model"
)

// engineRoleDTO — роль строки операции для мобильных клиентов.
type engineRoleDTO struct {
	// Role — семантическая роль («расход», «источник», …).
	Role string `json:"role"`
	// Side — сторона, которую назначит движок; пусто, если нужна direction.
	Side model.Side `json:"side,omitempty"`
	// AccountTypes — допустимые типы счетов для роли.
	AccountTypes []model.AccountType `json:"account_types"`
	// DirectionRequired — клиент обязан передать направление.
	DirectionRequired bool `json:"direction_required"`
	// Reserved — роль операции post-MVP; в MVP скрывается.
	Reserved bool `json:"reserved"`
}

// engineRolesResponse — описание ролей и сторон матрицы для типа операции.
type engineRolesResponse struct {
	OpType    model.OpType     `json:"op_type"`
	Title     string           `json:"title"`
	Roles     []engineRoleDTO  `json:"roles"`
	MinDebit  int              `json:"min_debit"`
	MinCredit int              `json:"min_credit"`
	Reserved  reservedCellsDTO `json:"reserved,omitempty"`
	// AccountsReadOnly напоминает, что план счетов меняется только на сервере.
	AccountsReadOnly bool `json:"accounts_read_only"`
}

// reservedCellsDTO — зарезервированные (post-MVP) типы счетов по сторонам.
type reservedCellsDTO struct {
	Debit  []model.AccountType `json:"debit,omitempty"`
	Credit []model.AccountType `json:"credit,omitempty"`
}

// handleEngineRoles отдаёт роли и стороны матрицы ASL-2.2 для клиента.
//
// Это единственный источник правил для мобильного приложения: клиент не
// дублирует матрицу, а группирует роли по стороне DEBIT/CREDIT.
func (s *Server) handleEngineRoles(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("op_type"))
	if raw == "" {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "нужен параметр op_type")
		return
	}
	op := model.OpType(raw)
	rule, ok := engine.RuleFor(op)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "ERR_OP_TYPE_UNKNOWN",
			"операция «"+raw+"» неизвестна")
		return
	}
	roles, ok := engine.RolesFor(op)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "ERR_OP_TYPE_UNKNOWN",
			"операция «"+raw+"» не поддерживает роли")
		return
	}

	items := make([]engineRoleDTO, 0, len(roles))
	for _, role := range roles {
		items = append(items, engineRoleDTO{
			Role:              string(role.Role),
			Side:              role.Side,
			AccountTypes:      role.AccountTypes,
			DirectionRequired: role.DirectionRequired,
			Reserved:          role.Reserved,
		})
	}

	writeJSON(w, http.StatusOK, engineRolesResponse{
		OpType:    op,
		Title:     rule.Title,
		Roles:     items,
		MinDebit:  rule.MinDebit,
		MinCredit: rule.MinCredit,
		Reserved: reservedCellsDTO{
			Debit:  rule.ReservedDebit,
			Credit: rule.ReservedCredit,
		},
		AccountsReadOnly: true,
	})
}
