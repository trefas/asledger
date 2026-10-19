package httpui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/export"
	"github.com/trefas/asledger/pkg/core/hledger"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/store"
)

// dataView — данные страницы импорта и экспорта книги.
type dataView struct {
	// Users — пользователи, один из них становится автором проводок импорта.
	Users []model.User
	// Mode — выбранный режим импорта.
	Mode string
	// Content — введённый текст книги (возвращается при ошибке).
	Content string
	// Error — сообщение об ошибке импорта.
	Error string
	// Report — отчёт последнего импорта.
	Report *importReport
}

// importReport — отчёт об импорте для страницы.
type importReport struct {
	Mode            string
	FileName        string
	Accepted        int
	Skipped         int
	Duplicates      int
	CreatedAccounts []string
	Rejected        bool
	Message         string
	Problems        []string
}

// handleData показывает страницу импорта и экспорта.
func (s *Server) handleData(w http.ResponseWriter, r *http.Request) {
	view, err := s.dataView(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, r, "data", pageData{Title: "Импорт и экспорт", Active: "data", Data: view})
}

// dataView собирает данные страницы импорта и экспорта.
func (s *Server) dataView(ctx context.Context) (dataView, error) {
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return dataView{}, err
	}
	return dataView{Users: users, Mode: string(hledger.ModeFlexible)}, nil
}

// handleDataImport импортирует книгу hledger, загруженную файлом или вставленную.
func (s *Server) handleDataImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		s.fail(w, err)
		return
	}

	content := strings.TrimSpace(r.FormValue("content"))
	fileName := strings.TrimSpace(r.FormValue("file_name"))
	if file, header, err := r.FormFile("file"); err == nil {
		defer func() { _ = file.Close() }()
		body, err := io.ReadAll(io.LimitReader(file, maxUploadBytes))
		if err != nil {
			s.fail(w, err)
			return
		}
		if len(strings.TrimSpace(string(body))) > 0 {
			content = string(body)
			if fileName == "" {
				fileName = header.Filename
			}
		}
	}

	mode, err := hledger.ParseMode(r.FormValue("mode"))
	if err != nil {
		s.dataFailure(w, r, err.Error(), content, r.FormValue("mode"), nil)
		return
	}
	if content == "" {
		s.dataFailure(w, r, "вставьте книгу hledger или выберите файл", "", r.FormValue("mode"), nil)
		return
	}

	actorID, err := s.importActor(ctx, r.FormValue("actor"))
	if err != nil {
		s.dataFailure(w, r, err.Error(), content, r.FormValue("mode"), nil)
		return
	}

	report, err := hledger.NewImporter(s.db).Import(ctx, []byte(content), hledger.Options{
		Mode: mode, ActorID: actorID, FileName: fileName,
	})
	if err != nil {
		s.dataFailure(w, r, "импорт не выполнен: "+err.Error(), content, r.FormValue("mode"), nil)
		return
	}
	s.dataFailure(w, r, "", content, string(mode), report)
}

// dataFailure повторно показывает страницу импорта с сообщением и отчётом.
func (s *Server) dataFailure(w http.ResponseWriter, r *http.Request, message, content, mode string, report *hledger.Report) {
	view, err := s.dataView(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	view.Error = message
	view.Content = content
	if parsed, err := hledger.ParseMode(mode); err == nil {
		view.Mode = string(parsed)
	}
	if report != nil {
		view.Report = importReportFrom(report)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	data := pageData{Title: "Импорт и экспорт", Active: "data", Version: s.cfg.Version, Data: view}
	if err := s.pages["data"].ExecuteTemplate(w, "layout", data); err != nil {
		s.logger.Error("не удалось отрисовать страницу импорта", "err", err)
	}
}

// importReportFrom переводит отчёт импортёра в вид страницы.
func importReportFrom(report *hledger.Report) *importReport {
	view := &importReport{
		Mode:            string(report.Mode),
		FileName:        report.FileName,
		Accepted:        report.Accepted,
		Skipped:         report.Skipped,
		Duplicates:      report.Duplicates,
		CreatedAccounts: report.CreatedAccounts,
		Rejected:        report.Rejected,
		Message:         report.Message,
	}
	for _, problem := range report.Problems {
		view.Problems = append(view.Problems, problem.Error())
	}
	return view
}

// importActor определяет автора проводок импорта.
//
// Пустое значение допустимо при единственном пользователе книги.
func (s *Server) importActor(ctx context.Context, value string) (int64, error) {
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return 0, err
	}
	if len(users) == 0 {
		return 0, errors.New("в книге нет пользователей: создайте пользователя на странице «Пользователи»")
	}
	if strings.TrimSpace(value) == "" {
		if len(users) == 1 {
			return users[0].ID, nil
		}
		return 0, errors.New("выберите автора проводок импорта")
	}
	for _, user := range users {
		if itoa(int(user.ID)) == value {
			return user.ID, nil
		}
	}
	return 0, errors.New("выбран неизвестный автор проводок")
}

// handleDataExport отдаёт книгу файлом: hledger, CSV журнала или отчёт.
func (s *Server) handleDataExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	format := r.PathValue("format")

	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		s.fail(w, err)
		return
	}

	var body bytes.Buffer
	switch format {
	case "hledger":
		if err := hledger.Export(ctx, s.db, &body, hledger.WriteOptions{
			Comment: "asledger: выгрузка книги",
		}); err != nil {
			s.fail(w, err)
			return
		}
		writeAttachment(w, "asledger.journal", "text/plain; charset=utf-8", body.Bytes())
	case "csv":
		if err := s.exportCSVReport(r, accounts, &body); err != nil {
			s.dataExportError(w, err)
			return
		}
		writeAttachment(w, "asledger.csv", "text/csv; charset=utf-8", body.Bytes())
	default:
		s.renderError(w, r, http.StatusNotFound, "Неизвестный формат выгрузки.")
	}
}

// exportCSVReport пишет CSV выбранного отчёта в буфер.
func (s *Server) exportCSVReport(r *http.Request, accounts []model.Account, body *bytes.Buffer) error {
	ctx := r.Context()
	switch report := r.URL.Query().Get("report"); report {
	case "", "journal":
		transactions, err := s.db.ListTransactions(ctx, store.TransactionFilter{})
		if err != nil {
			return err
		}
		return export.Journal(body, accounts, transactions)
	case "balances":
		balances, err := s.db.Balances(ctx)
		if err != nil {
			return err
		}
		return export.Balances(body, accounts, balances)
	case "turnover":
		from, to, err := webPeriod(r)
		if err != nil {
			return err
		}
		turnovers, err := s.db.Turnovers(ctx, from, to)
		if err != nil {
			return err
		}
		return export.Turnovers(body, accounts, turnovers)
	default:
		return errors.New("неизвестный отчёт " + report)
	}
}

// webPeriod разбирает период отчёта об оборотах из строки запроса.
func webPeriod(r *http.Request) (int64, int64, error) {
	query := r.URL.Query()
	var from, to int64
	if text := query.Get("from"); text != "" {
		date, err := time.Parse(time.DateOnly, text)
		if err != nil {
			return 0, 0, errors.New("неверная дата начала периода: ожидается ГГГГ-ММ-ДД")
		}
		from = date.UTC().Unix()
	}
	if text := query.Get("to"); text != "" {
		date, err := time.Parse(time.DateOnly, text)
		if err != nil {
			return 0, 0, errors.New("неверная дата конца периода: ожидается ГГГГ-ММ-ДД")
		}
		to = date.UTC().AddDate(0, 0, 1).Unix() - 1
	}
	return from, to, nil
}

// dataExportError сообщает об ошибке выгрузки понятной страницей.
func (s *Server) dataExportError(w http.ResponseWriter, err error) {
	s.logger.Error("не удалось выгрузить данные", "err", err)
	http.Error(w, "не удалось подготовить выгрузку", http.StatusInternalServerError)
}

// writeAttachment отправляет содержимое вложением.
func writeAttachment(w http.ResponseWriter, filename, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
