package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

func TestPickerItemFilterValueIncludesTitleAndDescription(t *testing.T) {
	item := pickerItem{
		title: "work",
		desc:  "7d 42% left reset 23 May 12:28",
		value: "opaque-value",
	}
	got := item.FilterValue()
	for _, want := range []string{"work", "42% left", "23 May"} {
		if !strings.Contains(got, want) {
			t.Fatalf("filter value missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, item.value) {
		t.Fatalf("filter value should not include opaque action value: %q", got)
	}
}

func TestPickerDelegateIgnoresUnexpectedItems(t *testing.T) {
	var out bytes.Buffer
	model := list.New([]list.Item{}, pickerDelegate{}, 40, 6)
	pickerDelegate{}.Render(&out, model, 0, nil)
	if out.Len() != 0 {
		t.Fatalf("unexpected render output for invalid item: %q", out.String())
	}
}

func TestPickerModelRendersNarrowAndNavigates(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	statuses := []AccountStatus{
		{Account: Account{Name: "work", Home: "/tmp/work"}, Auth: AuthInfo{LoggedIn: true, Email: "user@example.com"}},
	}
	model := newPickerModel(context.Background(), cfg, provider, statuses)

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 32, Height: 10})
	model = updated.(pickerModel)
	view := model.View()
	if !view.AltScreen {
		t.Fatal("picker should render in alt screen")
	}
	if !strings.Contains(view.Content, "enter: select") {
		t.Fatalf("expected account footer, got %q", view.Content)
	}
	if strings.Count(view.Content, "work") > 1 {
		t.Fatalf("picker account row should not repeat account name, got %q", view.Content)
	}

	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(pickerModel)
	if model.stage != stageAction {
		t.Fatalf("expected action stage, got %v", model.stage)
	}
	if !strings.Contains(model.View().Content, "resume here") {
		t.Fatalf("expected action list, got %q", model.View().Content)
	}

	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	model = updated.(pickerModel)
	if model.stage != stageAccount {
		t.Fatalf("expected account stage after esc, got %v", model.stage)
	}
}

func TestPickerModelCanNavigateBeforeWindowSize(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	statuses := []AccountStatus{
		{Account: Account{Name: "work", Home: "/tmp/work"}, Auth: AuthInfo{LoggedIn: true, Email: "user@example.com"}},
	}
	model := newPickerModel(context.Background(), cfg, provider, statuses)

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(pickerModel)
	if model.stage != stageAction {
		t.Fatalf("expected action stage, got %v", model.stage)
	}
	if !strings.Contains(model.View().Content, "resume here") {
		t.Fatalf("expected action list before window-size event, got %q", model.View().Content)
	}

	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	model = updated.(pickerModel)
	if model.stage != stageAccount {
		t.Fatalf("expected account stage after esc, got %v", model.stage)
	}
	if !strings.Contains(model.View().Content, "enter: select") {
		t.Fatalf("expected account list after back navigation, got %q", model.View().Content)
	}
}

func TestInitialPickerStatusesUseAuthInfoWithoutQuotaStatus(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}

	statuses := initialPickerStatuses(cfg, provider)
	if !provider.authCalled {
		t.Fatal("expected initial picker status to read local auth info")
	}
	if provider.statusCalled {
		t.Fatal("initial picker status should not fetch quota/status before UI starts")
	}
	if len(statuses) != 2 {
		t.Fatalf("expected statuses for configured accounts, got %#v", statuses)
	}
}

func TestPickerInitialLoadCommandFetchesStatuses(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	model := newPickerModel(context.Background(), cfg, provider, nil)
	model.load = true

	cmd := model.Init()
	if cmd == nil {
		t.Fatal("expected initial load command")
	}
	msg := cmd()
	statuses, ok := msg.(statusesMsg)
	if !ok {
		t.Fatalf("expected statusesMsg, got %#v", msg)
	}
	if !provider.statusCalled {
		t.Fatal("expected load command to fetch provider statuses")
	}
	if statuses.refreshed {
		t.Fatal("initial load should use cached/non-refresh status mode")
	}
}

func TestPickerStatusUpdateDoesNotReplaceActionList(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	statuses := []AccountStatus{
		{Account: Account{Name: "work", Home: "/tmp/work"}, Auth: AuthInfo{LoggedIn: true, Email: "old@example.com"}},
	}
	model := newPickerModel(context.Background(), cfg, provider, statuses)

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(pickerModel)
	if model.stage != stageAction {
		t.Fatalf("expected action stage, got %v", model.stage)
	}

	updated, _ = model.Update(statusesMsg{statuses: []AccountStatus{
		{Account: Account{Name: "work", Home: "/tmp/work"}, Auth: AuthInfo{LoggedIn: true, Email: "new@example.com"}},
	}})
	model = updated.(pickerModel)
	if model.stage != stageAction {
		t.Fatalf("expected to stay in action stage, got %v", model.stage)
	}
	if !strings.Contains(model.View().Content, "resume here") {
		t.Fatalf("status update replaced action list: %q", model.View().Content)
	}
	if model.selected.Auth.Email != "new@example.com" {
		t.Fatalf("selected account did not update with loaded status: %#v", model.selected)
	}
}

func TestRunPickerResultLaunchesSelectedAction(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	cfg := providerTestConfig(t)
	account, ok := cfg.Account("work")
	if !ok {
		t.Fatal("expected work account")
	}

	tests := []struct {
		name       string
		action     pickerAction
		wantArgs   []string
		resumeAll  bool
		resumeLast bool
	}{
		{name: "new", action: actionNew, wantArgs: []string{"new", cwd}},
		{name: "resume here", action: actionResume, wantArgs: []string{"resume", cwd, ""}},
		{name: "resume all", action: actionResumeAll, wantArgs: []string{"resume", "", ""}, resumeAll: true},
		{name: "login", action: actionLogin, wantArgs: []string{"login"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := &fakeProvider{}
			err := runPickerResult(cfg, provider, pickerResult{account: account, action: tc.action})
			if err != nil {
				t.Fatal(err)
			}
			if !provider.initCalled {
				t.Fatal("picker launch should initialize the selected account home")
			}
			if provider.launch == nil || provider.launch.Account.Name != "work" {
				t.Fatalf("picker launch used wrong account: %#v", provider.launch)
			}
			if !equalStrings(provider.launch.Args, tc.wantArgs) {
				t.Fatalf("launch args mismatch: got %#v want %#v", provider.launch.Args, tc.wantArgs)
			}
			if provider.resumeAll != tc.resumeAll || provider.resumeLast != tc.resumeLast {
				t.Fatalf("resume flags mismatch: last=%v all=%v", provider.resumeLast, provider.resumeAll)
			}
		})
	}
}
