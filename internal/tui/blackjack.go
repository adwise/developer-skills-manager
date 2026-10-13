package tui

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

type blackjack struct {
	deck   []int
	player []int
	dealer []int
	result string
}

func newBlackjack() blackjack {
	deck := rand.Perm(52)
	for i := range deck {
		deck[i] = deck[i]%13 + 1
	}
	return dealBlackjack(deck)
}

func dealBlackjack(deck []int) blackjack {
	game := blackjack{deck: deck}
	for range 2 {
		game.player = append(game.player, game.draw())
		game.dealer = append(game.dealer, game.draw())
	}
	if blackjackScore(game.player) == 21 || blackjackScore(game.dealer) == 21 {
		game.settle()
	}
	return game
}

func (g *blackjack) draw() int {
	card := g.deck[0]
	g.deck = g.deck[1:]
	return card
}

func blackjackScore(hand []int) int {
	total, aces := 0, 0
	for _, card := range hand {
		switch {
		case card == 1:
			total += 11
			aces++
		case card > 10:
			total += 10
		default:
			total += card
		}
	}
	for total > 21 && aces > 0 {
		total -= 10
		aces--
	}
	return total
}

func (g *blackjack) play(key string) {
	if g.result != "" {
		if key == "n" {
			*g = newBlackjack()
		}
		return
	}
	switch key {
	case "h":
		g.player = append(g.player, g.draw())
		if blackjackScore(g.player) > 21 {
			g.result = "Bust! Dealer wins."
		} else if blackjackScore(g.player) == 21 {
			g.stand()
		}
	case "s":
		g.stand()
	}
}

func (g *blackjack) stand() {
	// Dealer stands on all 17s, including soft 17.
	for blackjackScore(g.dealer) < 17 {
		g.dealer = append(g.dealer, g.draw())
	}
	g.settle()
}

func (g *blackjack) settle() {
	player, dealer := blackjackScore(g.player), blackjackScore(g.dealer)
	switch {
	case player > 21:
		g.result = "Bust! Dealer wins."
	case dealer > 21 || player > dealer:
		g.result = "You win!"
	case player < dealer:
		g.result = "Dealer wins."
	default:
		g.result = "Push! It's a tie."
	}
}

func blackjackHand(hand []int) string {
	ranks := []string{"", "A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
	cards := make([]string, len(hand))
	for i, card := range hand {
		cards[i] = ranks[card]
	}
	return strings.Join(cards, " ")
}

func (g blackjack) view() string {
	dealer := blackjackHand(g.dealer[:1]) + " [?]"
	controls := "h hit • s stand • q/esc back to skills"
	if g.result != "" {
		dealer = fmt.Sprintf("%s (%d)", blackjackHand(g.dealer), blackjackScore(g.dealer))
		controls = g.result + "\n\nn new game • q/esc back to skills"
	}
	return fmt.Sprintf("Blackjack and hookers\n(just blackjack — no money, no stakes)\n\nDealer: %s\nYou:    %s (%d)\n\n%s\n", dealer, blackjackHand(g.player), blackjackScore(g.player), controls)
}
