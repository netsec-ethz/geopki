package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func QueryState(
	key string,
	transaction pgx.Tx,
	ctx context.Context,
) (string, error) {

	preparedStatementName := fmt.Sprintf("state:%s", key)

	_, err := transaction.Prepare(
		ctx,
		preparedStatementName,
		"SELECT value FROM state WHERE key=$1::text",
	)
	if err != nil {
		return "", err
	}

	var value string
	err = transaction.QueryRow(ctx, preparedStatementName, key).Scan(&value)
	if err != nil {
		return "", err
	}

	return value, nil
}

func UpdateState(
	key string,
	oldValue string,
	newValue string,
	transaction pgx.Tx,
	ctx context.Context,
) (bool, error) {

	preparedStatementName := fmt.Sprintf("update-state:%s", key)

	_, err := transaction.Prepare(
		ctx,
		preparedStatementName,
		"UPDATE state SET value=$1::text WHERE key=$2::text AND value=$3::text",
	)
	if err != nil {
		return false, err
	}

	tag, err := transaction.Exec(ctx, preparedStatementName, newValue, key, oldValue)
	if err != nil {
		return false, err
	}

	rowsAffected := tag.RowsAffected()
	if rowsAffected > 1 {
		return true, fmt.Errorf("more than one row was affected in a state update")
	}

	return rowsAffected == 1, nil
}
