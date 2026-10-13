package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adwise/developer-skills-manager/internal/skill"
)

func TestBlackjackScore(t *testing.T) {
	for _, test := range []struct {
		hand []int
		want int
	}{
		{[]int{1, 13}, 21},
		{[]int{1, 1, 9}, 21},
		{[]int{1, 1, 13, 9}, 21},
		{[]int{11, 12, 13}, 30},
		{[]int{1, 6}, 17},
	} {
		if got := blackjackScore(test.hand); got != test.want {
			t.Errorf("score(%v) = %d, want %d", test.hand, got, test.want)
		}
	}
}

func TestBlackjackRounds(t *testing.T) {
	for _, test := range []struct {
		name   string
		deck   []int
		keys   string
		result string
	}{
		{"player blackjack", []int{1, 10, 13, 7}, "", "You win!"},
		{"dealer blackjack", []int{10, 1, 7, 13}, "", "Dealer wins."},
		{"both blackjack", []int{1, 1, 13, 12}, "", "Push! It's a tie."},
		{"hit bust", []int{10, 10, 6, 7, 13}, "h", "Bust! Dealer wins."},
		{"hit 21", []int{10, 10, 6, 7, 5}, "h", "You win!"},
		{"dealer draws and busts", []int{10, 10, 8, 6, 13}, "s", "You win!"},
		{"dealer wins", []int{10, 10, 7, 9}, "s", "Dealer wins."},
		{"tie", []int{10, 10, 8, 8}, "s", "Push! It's a tie."},
		{"dealer stands on soft 17", []int{10, 1, 8, 6}, "s", "You win!"},
	} {
		t.Run(test.name, func(t *testing.T) {
			game := dealBlackjack(test.deck)
			for _, key := range test.keys {
				game.play(string(key))
			}
			if game.result != test.result {
				t.Fatalf("result = %q, want %q", game.result, test.result)
			}
			before := append([]int(nil), game.player...)
			game.play("h")
			if !reflect.DeepEqual(game.player, before) {
				t.Fatal("finished game accepted a hit")
			}
			game.play("n")
			if len(game.player) != 2 || len(game.dealer) != 2 || len(game.deck) != 48 {
				t.Fatal("new game did not deal a fresh deck")
			}
		})
	}
}

func TestBlackjackDeck(t *testing.T) {
	game := newBlackjack()
	counts := make(map[int]int)
	for _, hand := range [][]int{game.player, game.dealer, game.deck} {
		for _, card := range hand {
			counts[card]++
		}
	}
	for rank := 1; rank <= 13; rank++ {
		if counts[rank] != 4 {
			t.Fatalf("rank %d occurs %d times", rank, counts[rank])
		}
	}
}

func TestBlackjackViewHidesDealerUntilFinished(t *testing.T) {
	game := dealBlackjack([]int{10, 9, 8, 8})
	if !strings.Contains(game.view(), "Dealer: 9 [?]") {
		t.Fatal("dealer's second card is not hidden")
	}
	game.play("s")
	if !strings.Contains(game.view(), "Dealer: 9 8 (17)") {
		t.Fatal("finished game did not reveal dealer's hand")
	}
}

func TestInstallerBlackjackPreservesSelection(t *testing.T) {
	for _, back := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyEsc},
	} {
		model := NewInstallerModel([]skill.Metadata{{ID: "test"}}, nil, nil)
		model.selected["test"] = true
		updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
		model = updated.(InstallerModel)
		if !model.playing || cmd != nil || !strings.Contains(model.View(), "Blackjack and hookers") {
			t.Fatal("blackjack did not open")
		}
		updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = updated.(InstallerModel)
		if model.complete || cmd != nil {
			t.Fatal("game applied installer selection")
		}
		updated, cmd = model.Update(back)
		model = updated.(InstallerModel)
		if model.playing || model.Cancelled() || cmd != nil || !reflect.DeepEqual(model.Selected(), []string{"test"}) {
			t.Fatal("returning from blackjack changed installer state")
		}
	}
}

func TestInstallerBlackjackCtrlCQuits(t *testing.T) {
	model := NewInstallerModel(nil, nil, nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	updated, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !updated.(InstallerModel).Cancelled() || cmd == nil {
		t.Fatal("ctrl+c did not cancel the installer from blackjack")
	}
}
