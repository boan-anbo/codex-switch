package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type pickerStage int

const (
	stageAccount pickerStage = iota
	stageAction
)

type pickerAction string

const (
	actionNew       pickerAction = "new"
	actionResume    pickerAction = "resume"
	actionResumeAll pickerAction = "resume-all"
	actionLogin     pickerAction = "login"
)

type pickerResult struct {
	account Account
	action  pickerAction
}

type pickerItem struct {
	title string
	desc  string
	value string
}

func (item pickerItem) FilterValue() string {
	return item.title + " " + item.desc
}

type pickerDelegate struct{}

func (pickerDelegate) Height() int {
	return 2
}

func (pickerDelegate) Spacing() int {
	return 0
}

func (pickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (pickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	pickerItem, ok := item.(pickerItem)
	if !ok {
		return
	}
	titleStyle := lipgloss.NewStyle().Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	prefix := "  "
	if index == m.Index() {
		prefix = "> "
		titleStyle = titleStyle.Foreground(lipgloss.Color("205"))
	}
	fmt.Fprint(w, titleStyle.Render(prefix+pickerItem.title))
	if pickerItem.desc != "" {
		fmt.Fprint(w, "\n"+descStyle.Render("  "+pickerItem.desc))
	}
}

type pickerModel struct {
	ctx      context.Context
	cfg      *Config
	provider Provider
	stage    pickerStage
	list     list.Model
	statuses []AccountStatus
	selected AccountStatus
	result   *pickerResult
	message  string
	width    int
	height   int
	load     bool
}

type statusesMsg struct {
	statuses  []AccountStatus
	refreshed bool
}

func RunPicker(ctx context.Context, cfg *Config, provider Provider) error {
	if err := ensureRuntimeAccountHomesUnique(cfg); err != nil {
		return err
	}
	statuses := initialPickerStatuses(cfg, provider)
	model := newPickerModel(ctx, cfg, provider, statuses)
	model.message = "loading usage..."
	model.load = true
	program := tea.NewProgram(model)
	final, err := program.Run()
	if err != nil {
		return err
	}
	m, ok := final.(pickerModel)
	if !ok || m.result == nil {
		return nil
	}
	return runPickerResult(cfg, provider, *m.result)
}

func runPickerResult(cfg *Config, provider Provider, result pickerResult) error {
	cwd, _ := os.Getwd()
	switch result.action {
	case actionNew:
		args, err := provider.NewArgs(cfg, "", cwd, nil)
		if err != nil {
			return err
		}
		return launchAccountCodex(provider, cfg, LaunchOptions{Account: result.account, Args: args})
	case actionResume:
		args, err := provider.ResumeArgs(cfg, "", cwd, "", false, false, nil)
		if err != nil {
			return err
		}
		return launchAccountCodex(provider, cfg, LaunchOptions{Account: result.account, Args: args})
	case actionResumeAll:
		args, err := provider.ResumeArgs(cfg, "", "", "", false, true, nil)
		if err != nil {
			return err
		}
		return launchAccountCodex(provider, cfg, LaunchOptions{Account: result.account, Args: args})
	case actionLogin:
		return launchAccountCodex(provider, cfg, LaunchOptions{Account: result.account, Args: []string{"login"}})
	default:
		return nil
	}
}

func newPickerModel(ctx context.Context, cfg *Config, provider Provider, statuses []AccountStatus) pickerModel {
	items := make([]list.Item, 0, len(statuses))
	for _, status := range statuses {
		items = append(items, pickerAccountItem(status))
	}
	l := list.New(items, pickerDelegate{}, pickerListWidth(0), pickerListHeight(0))
	l.Title = "Pick Codex account"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	return pickerModel{ctx: ctx, cfg: cfg, provider: provider, stage: stageAccount, list: l, statuses: statuses}
}

func (m pickerModel) Init() tea.Cmd {
	if m.load {
		return m.loadStatuses(false)
	}
	return nil
}

func initialPickerStatuses(cfg *Config, provider Provider) []AccountStatus {
	accounts := cfg.AccountsList()
	statuses := make([]AccountStatus, 0, len(accounts))
	for _, account := range accounts {
		statuses = append(statuses, AccountStatus{Account: account, Auth: provider.AuthInfo(account)})
	}
	return statuses
}

func (m pickerModel) loadStatuses(refresh bool) tea.Cmd {
	return func() tea.Msg {
		return statusesMsg{statuses: m.provider.Statuses(m.ctx, m.cfg, refresh), refreshed: refresh}
	}
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(pickerListWidth(m.width), pickerListHeight(m.height))
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			if m.stage == stageAction {
				return m.backToAccounts(), nil
			}
			return m, tea.Quit
		case "r":
			if m.stage == stageAccount {
				m.message = "refreshing usage..."
				return m, m.loadStatuses(true)
			}
		case "enter":
			selected, ok := m.list.SelectedItem().(pickerItem)
			if !ok {
				break
			}
			if m.stage == stageAccount {
				for _, status := range m.statuses {
					if status.Account.Name == selected.value {
						m.selected = status
						m.stage = stageAction
						m.list = actionList(status)
						m.list.SetSize(pickerListWidth(m.width), pickerListHeight(m.height))
						return m, nil
					}
				}
			} else {
				m.result = &pickerResult{account: m.selected.Account, action: pickerAction(selected.value)}
				return m, tea.Quit
			}
		}
	case statusesMsg:
		m.statuses = msg.statuses
		m.message = "usage loaded"
		if msg.refreshed {
			m.message = "usage refreshed"
		}
		if m.stage == stageAction {
			for _, status := range m.statuses {
				if status.Account.Name == m.selected.Account.Name {
					m.selected = status
					break
				}
			}
			break
		}
		m.list.SetItems(pickerStatusItems(m.statuses))
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m pickerModel) View() tea.View {
	footer := "enter: select  /: filter  r: refresh  q: quit"
	if m.stage == stageAction {
		footer = "enter: run  esc/q: back"
	}
	body := m.list.View()
	if m.message != "" {
		body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(m.message)
	}
	view := tea.NewView(strings.TrimRight(body, "\n") + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(footer))
	view.AltScreen = true
	return view
}

func (m pickerModel) backToAccounts() pickerModel {
	m.stage = stageAccount
	m.list = list.New(pickerStatusItems(m.statuses), pickerDelegate{}, pickerListWidth(m.width), pickerListHeight(m.height))
	m.list.Title = "Pick Codex account"
	m.list.SetShowStatusBar(false)
	m.list.SetFilteringEnabled(true)
	m.list.SetShowHelp(true)
	return m
}

func pickerStatusItems(statuses []AccountStatus) []list.Item {
	items := make([]list.Item, 0, len(statuses))
	for _, status := range statuses {
		items = append(items, pickerAccountItem(status))
	}
	return items
}

func pickerAccountItem(status AccountStatus) pickerItem {
	return pickerItem{
		title: accountDisplayName(status.Account),
		desc:  formatAccountDetails(status),
		value: status.Account.Name,
	}
}

func actionList(status AccountStatus) list.Model {
	items := []list.Item{
		pickerItem{title: "new", desc: "Start a new Codex session under " + status.Account.Name, value: string(actionNew)},
		pickerItem{title: "resume here", desc: "Open Codex resume scoped to this working directory", value: string(actionResume)},
		pickerItem{title: "resume all", desc: "Open Codex resume across directories, including non-interactive sessions", value: string(actionResumeAll)},
		pickerItem{title: "login / refresh", desc: "Run codex login with CODEX_HOME set to this account", value: string(actionLogin)},
	}
	l := list.New(items, pickerDelegate{}, 96, 12)
	l.Title = "Pick action for " + status.Account.Name
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)
	return l
}

func pickerListWidth(width int) int {
	if width > 0 {
		return max(40, width)
	}
	return 96
}

func pickerListHeight(height int) int {
	if height > 0 {
		return max(10, height-4)
	}
	return 18
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
