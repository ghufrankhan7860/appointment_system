package repositories

import (
	"context"
	"testing"

	"gorm.io/gorm"
)

func TestWithTxRoundTrip(t *testing.T) {
	tx := &gorm.DB{}
	ctx := withTx(context.Background(), tx)

	got, ok := txFromContext(ctx)
	if !ok {
		t.Fatal("txFromContext returned ok=false for a context carrying a transaction")
	}
	if got != tx {
		t.Fatal("txFromContext did not return the stored transaction")
	}
}

func TestTxFromContextEmpty(t *testing.T) {
	if _, ok := txFromContext(context.Background()); ok {
		t.Fatal("txFromContext returned ok=true for a context without a transaction")
	}
}
