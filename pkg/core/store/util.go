package store

import "strings"

// nullableID превращает необязательный идентификатор в значение для SQL.
func nullableID(id *int64) any {
	if id == nil {
		return nil
	}
	return *id
}

// isUniqueViolation сообщает, вызвана ли ошибка нарушением уникальности.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// escapeLike экранирует спецсимволы шаблона LIKE.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
