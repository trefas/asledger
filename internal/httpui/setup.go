package httpui

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/store"
)

// maxUploadBytes — предельный размер загружаемого файла плана счетов.
const maxUploadBytes = 4 << 20

// setupView — данные страницы первичной настройки.
type setupView struct {
	// Templates — шаблонные планы счетов из каталога.
	Templates []chartTemplate
	// Users — уже созданные пользователи семьи.
	Users []model.User
	// NeedUser — нужно создать первого пользователя.
	NeedUser bool
	// Error — сообщение об ошибке импорта.
	Error string
	// Content — введённый JSON (возвращается при ошибке).
	Content string
}

// chartTemplate — шаблонный план счетов.
type chartTemplate struct {
	File     string
	Name     string
	Accounts int
}

// handleSetupForm показывает экран первичной настройки.
func (s *Server) handleSetupForm(w http.ResponseWriter, r *http.Request) {
	view, err := s.setupView(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, r, "setup", pageData{Title: "Настройка", Active: "setup", Data: view})
}

// setupView собирает данные экрана настройки.
func (s *Server) setupView(ctx context.Context) (setupView, error) {
	templates, err := s.chartTemplates()
	if err != nil {
		return setupView{}, err
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return setupView{}, err
	}
	view := setupView{Templates: templates, NeedUser: len(users) == 0, Users: users}
	return view, nil
}

// chartTemplates читает шаблоны планов счетов из каталога.
func (s *Server) chartTemplates() ([]chartTemplate, error) {
	files, err := filepath.Glob(filepath.Join(s.cfg.ChartsDir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	templates := make([]chartTemplate, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		chart, err := store.ParseChart(data)
		if err != nil {
			continue
		}
		name := chart.Name
		if name == "" {
			name = filepath.Base(file)
		}
		templates = append(templates, chartTemplate{
			File:     filepath.Base(file),
			Name:     name,
			Accounts: countChartNodes(chart),
		})
	}
	return templates, nil
}

// handleSetupTemplate создаёт план счетов из выбранного шаблона.
func (s *Server) handleSetupTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}

	file := r.FormValue("template")
	if file == "" {
		s.setupFailure(w, r, "выберите шаблон плана счетов", "")
		return
	}
	if err := s.ensureFirstUser(ctx, r); err != nil {
		s.setupFailure(w, r, err.Error(), "")
		return
	}

	data, err := os.ReadFile(filepath.Join(s.cfg.ChartsDir, filepath.Base(file)))
	if err != nil {
		s.setupFailure(w, r, "не удалось прочитать шаблон: "+err.Error(), "")
		return
	}
	chart, err := store.ParseChart(data)
	if err != nil {
		s.setupFailure(w, r, "неверный формат шаблона: "+err.Error(), "")
		return
	}
	report, err := s.db.ImportChart(ctx, chart)
	if err != nil {
		s.setupFailure(w, r, "не удалось импортировать план счетов: "+statusMessage(err), "")
		return
	}
	s.redirectOK(w, r, "/", flashImport(report))
}

// handleSetupImport импортирует план счетов из введённого JSON или файла.
func (s *Server) handleSetupImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		s.fail(w, err)
		return
	}

	content := strings.TrimSpace(r.FormValue("content"))
	if file, _, err := r.FormFile("file"); err == nil {
		defer func() { _ = file.Close() }()
		body, err := io.ReadAll(io.LimitReader(file, maxUploadBytes))
		if err != nil {
			s.fail(w, err)
			return
		}
		if len(strings.TrimSpace(string(body))) > 0 {
			content = string(body)
		}
	}
	if content == "" {
		s.setupFailure(w, r, "вставьте JSON плана счетов или выберите файл", "")
		return
	}

	if err := s.ensureFirstUser(ctx, r); err != nil {
		s.setupFailure(w, r, err.Error(), content)
		return
	}

	chart, err := store.ParseChart([]byte(content))
	if err != nil {
		s.setupFailure(w, r, "неверный формат JSON: "+err.Error(), content)
		return
	}
	if err := store.ValidateChart(chart); err != nil {
		s.setupFailure(w, r, "план счетов не прошёл проверку: "+err.Error(), content)
		return
	}
	report, err := s.db.ImportChart(ctx, chart)
	if err != nil {
		s.setupFailure(w, r, "не удалось импортировать план счетов: "+statusMessage(err), content)
		return
	}
	s.redirectOK(w, r, "/", flashImport(report))
}

// ensureFirstUser создаёт первого пользователя семьи, если книга ещё пуста
// по пользователям. Имя и секрет берутся из формы настройки.
func (s *Server) ensureFirstUser(ctx context.Context, r *http.Request) error {
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return nil
	}
	name := strings.TrimSpace(r.FormValue("user_name"))
	secret := r.FormValue("user_secret")
	if name == "" || secret == "" {
		return errors.New("укажите имя и секрет первого пользователя семьи")
	}
	_, err = s.db.CreateUser(ctx, name, secret)
	return err
}

// setupFailure повторно показывает экран настройки с сообщением об ошибке.
func (s *Server) setupFailure(w http.ResponseWriter, r *http.Request, message, content string) {
	view, err := s.setupView(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	view.Error = message
	view.Content = content
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	data := pageData{Title: "Настройка", Active: "setup", Version: s.cfg.Version, Data: view}
	if err := s.pages["setup"].ExecuteTemplate(w, "layout", data); err != nil {
		s.logger.Error("не удалось отрисовать страницу настройки", "err", err)
	}
}

// redirectOK переадресует на страницу с сообщением об успехе.
func (s *Server) redirectOK(w http.ResponseWriter, r *http.Request, path, message string) {
	http.Redirect(w, r, path+"?flash="+encodeQuery(message)+"&kind=ok", http.StatusSeeOther)
}

// flashImport формирует сообщение об итоге импорта плана счетов.
func flashImport(report store.ImportReport) string {
	if report.Created == 0 {
		return "План счетов уже был создан ранее"
	}
	return "План счетов создан: счетов " + itoa(report.Created)
}

// countChartNodes считает вершины плана счетов.
func countChartNodes(chart *store.Chart) int {
	total := 0
	var walk func(nodes []store.ChartNode)
	walk = func(nodes []store.ChartNode) {
		for _, node := range nodes {
			total++
			walk(node.Children)
		}
	}
	walk(chart.Accounts)
	return total
}
