package botenv

import (
	"testing"
)

func TestGetEnvVars_AllBotTypes(t *testing.T) {
	for _, bt := range AllBotTypes() {
		vars := GetEnvVars(bt)
		if len(vars) == 0 {
			t.Errorf("GetEnvVars(%q) returned 0 vars", bt)
		}
	}
}

func TestGetEnvVars_Unknown(t *testing.T) {
	vars := GetEnvVars("nonexistent")
	if vars != nil {
		t.Errorf("GetEnvVars(nonexistent) = %v, want nil", vars)
	}
}

func TestAllBotTypes(t *testing.T) {
	types := AllBotTypes()
	if len(types) != 4 {
		t.Errorf("AllBotTypes() returned %d types, want 4", len(types))
	}
	expected := map[BotType]bool{PicoClaw: true, IronClaw: true, OpenClaw: true, BusyBox: true}
	for _, bt := range types {
		if !expected[bt] {
			t.Errorf("unexpected bot type: %q", bt)
		}
	}
}

func TestByCategory(t *testing.T) {
	vars := GetEnvVars(PicoClaw)

	llm := ByCategory(vars, CategoryLLM)
	if len(llm) == 0 {
		t.Error("PicoClaw should have LLM vars")
	}
	for _, v := range llm {
		if v.Category != CategoryLLM {
			t.Errorf("ByCategory(LLM) returned var with category %q", v.Category)
		}
	}

	channels := ByCategory(vars, CategoryChannel)
	if len(channels) == 0 {
		t.Error("PicoClaw should have channel vars")
	}

	tools := ByCategory(vars, CategoryTool)
	if len(tools) == 0 {
		t.Error("PicoClaw should have tool vars")
	}

	system := ByCategory(vars, CategorySystem)
	if len(system) == 0 {
		t.Error("PicoClaw should have system vars")
	}

	// Empty result for nonexistent category
	empty := ByCategory(vars, "nonexistent")
	if len(empty) != 0 {
		t.Errorf("ByCategory(nonexistent) returned %d vars", len(empty))
	}
}

func TestByChannel(t *testing.T) {
	vars := GetEnvVars(PicoClaw)

	discord := ByChannel(vars, "discord")
	if len(discord) == 0 {
		t.Error("PicoClaw should have discord vars")
	}
	for _, v := range discord {
		if v.Channel != "discord" {
			t.Errorf("ByChannel(discord) returned var with channel %q", v.Channel)
		}
	}

	telegram := ByChannel(vars, "telegram")
	if len(telegram) == 0 {
		t.Error("PicoClaw should have telegram vars")
	}

	empty := ByChannel(vars, "nonexistent")
	if len(empty) != 0 {
		t.Errorf("ByChannel(nonexistent) returned %d vars", len(empty))
	}
}

func TestSecrets(t *testing.T) {
	vars := GetEnvVars(PicoClaw)
	secrets := Secrets(vars)
	if len(secrets) == 0 {
		t.Error("PicoClaw should have secret vars")
	}
	for _, v := range secrets {
		if !v.Secret {
			t.Errorf("Secrets() returned non-secret var: %q", v.Name)
		}
	}

	// All non-secrets should be excluded
	nonSecrets := 0
	for _, v := range vars {
		if !v.Secret {
			nonSecrets++
		}
	}
	if len(secrets)+nonSecrets != len(vars) {
		t.Errorf("secret(%d) + non-secret(%d) != total(%d)", len(secrets), nonSecrets, len(vars))
	}
}

func TestByCategory_Empty(t *testing.T) {
	result := ByCategory(nil, CategoryLLM)
	if len(result) != 0 {
		t.Errorf("ByCategory(nil) returned %d", len(result))
	}
}

func TestByChannel_Empty(t *testing.T) {
	result := ByChannel(nil, "discord")
	if len(result) != 0 {
		t.Errorf("ByChannel(nil) returned %d", len(result))
	}
}

func TestSecrets_Empty(t *testing.T) {
	result := Secrets(nil)
	if len(result) != 0 {
		t.Errorf("Secrets(nil) returned %d", len(result))
	}
}
