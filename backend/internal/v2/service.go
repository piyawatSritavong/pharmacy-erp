package v2

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
	_ "time/tzdata"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

const maxInteger int64 = 9_007_199_254_740_991

type Service struct{ db *sql.DB }
type Actor struct {
	User        platform.AuthUser
	OperationID string
}
type UnitSnapshot struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Conversion        int64  `json:"conversion"`
	Quantity          int64  `json:"quantity"`
	BaseQuantity      int64  `json:"base_quantity"`
	DefaultPriceCents int64  `json:"default_price_cents"`
}

func bad(message string) error      { return platform.NewError(http.StatusBadRequest, message) }
func conflict(message string) error { return platform.NewError(http.StatusConflict, message) }
func newID() string                 { return platform.MustUUID() }
func optionalID(id string) any {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return id
}
func rawJSON(value any) string { raw, _ := json.Marshal(value); return string(raw) }
func today() string {
	zone, _ := time.LoadLocation("Asia/Bangkok")
	return time.Now().In(zone).Format("2006-01-02")
}
func date(value string) error {
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return bad("วันที่ต้องอยู่ในรูป YYYY-MM-DD")
	}
	return nil
}
func proportion(amount, numerator, denominator int64) (int64, error) {
	if amount < 0 || numerator < 0 || denominator <= 0 {
		return 0, bad("จำนวนหรือต้นทุนไม่ถูกต้อง")
	}
	x := new(big.Int).Mul(big.NewInt(amount), big.NewInt(numerator))
	x.Quo(x, big.NewInt(denominator))
	if !x.IsInt64() || x.Int64() > maxInteger {
		return 0, bad("จำนวนเงินเกินขอบเขต")
	}
	return x.Int64(), nil
}
func multiply(a, b int64) (int64, error) { return proportion(a, b, 1) }
func add(a, b int64) (int64, error) {
	if a < 0 || b < 0 || a > maxInteger-b {
		return 0, bad("ยอดรวมเกินขอบเขต")
	}
	return a + b, nil
}

func branch(ctx context.Context, tx platform.DBTX, user platform.AuthUser, requested string) (string, error) {
	id, err := platform.MustBranchID(user, requested)
	if err != nil {
		return "", err
	}
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT active FROM branches WHERE id=$1`, id).Scan(&active); err != nil {
		return "", err
	}
	if !active {
		return "", conflict("สาขาถูกปิดใช้งาน")
	}
	return id, nil
}

func resolveUnit(ctx context.Context, tx platform.DBTX, productID, unitID string, quantity int64) (UnitSnapshot, error) {
	u := UnitSnapshot{Quantity: quantity, Conversion: 1}
	if quantity <= 0 || quantity > maxInteger {
		return u, bad("จำนวนต้องเป็นจำนวนเต็มบวก")
	}
	if err := tx.QueryRowContext(ctx, `SELECT unit_name,ROUND(base_selling_price*100)::bigint FROM products WHERE id=$1 AND active=TRUE`, productID).Scan(&u.Name, &u.DefaultPriceCents); err != nil {
		return u, conflict("ไม่พบสินค้าที่เปิดใช้งาน")
	}
	if unitID != "" {
		if err := tx.QueryRowContext(ctx, `SELECT id::text,unit_name,conversion_qty,ROUND(COALESCE(selling_price,$3::numeric/100*conversion_qty)*100)::bigint FROM product_units WHERE id=$1 AND product_id=$2 AND active=TRUE`, unitID, productID, u.DefaultPriceCents).Scan(&u.ID, &u.Name, &u.Conversion, &u.DefaultPriceCents); err != nil {
			return u, conflict("หน่วยขายไม่พร้อมใช้งาน")
		}
	}
	if u.Conversion <= 0 || u.DefaultPriceCents < 0 || u.DefaultPriceCents > maxInteger {
		return u, bad("หน่วยหรือราคาจาก catalog ไม่ถูกต้อง")
	}
	var err error
	u.BaseQuantity, err = multiply(quantity, u.Conversion)
	return u, err
}

func (s *Service) execute(ctx context.Context, user platform.AuthUser, action, key string, input any, fn func(*sql.Tx, Actor) (any, error)) (any, error) {
	key = strings.TrimSpace(key)
	if len(key) < 8 || len(key) > 128 {
		return nil, bad("ต้องส่ง Idempotency-Key ความยาว 8–128 ตัวอักษร")
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(rawJSON(input))))
	var result any
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		id := newID()
		var existingHash string
		var cached []byte
		_, err := tx.ExecContext(ctx, `INSERT INTO v2_operations(id,actor_id,action,request_key,request_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT(actor_id,action,request_key) DO NOTHING`, id, user.ID, action, key, hash)
		if err != nil {
			return err
		}
		if err = tx.QueryRowContext(ctx, `SELECT id::text,request_hash,result FROM v2_operations WHERE actor_id=$1 AND action=$2 AND request_key=$3 FOR UPDATE`, user.ID, action, key).Scan(&id, &existingHash, &cached); err != nil {
			return err
		}
		if existingHash != hash {
			return conflict("key เดิมถูกใช้กับข้อมูลชุดอื่นแล้ว")
		}
		if len(cached) > 0 {
			decoder := json.NewDecoder(strings.NewReader(string(cached)))
			decoder.UseNumber()
			return decoder.Decode(&result)
		}
		// Serialize finance in a branch so shift acknowledgements cannot race
		// a new refund/payment. The account lock also protects cross-branch opens.
		for _, prefix := range []string{"document.", "quotation.", "credit.", "payment.", "cheque.", "drawer.", "invoice.", "refund.", "shift."} {
			if !strings.HasPrefix(action, prefix) {
				continue
			}
			var scope BranchInput
			if err = json.Unmarshal([]byte(rawJSON(input)), &scope); err != nil {
				return err
			}
			b, e := branch(ctx, tx, user, scope.BranchID)
			if e != nil {
				return e
			}
			if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('v2:account:'||$1,0))`, user.ID); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('v2:finance:'||$1,0))`, b); err != nil {
				return err
			}
			break
		}
		result, err = fn(tx, Actor{User: user, OperationID: id})
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE v2_operations SET result=$2::jsonb WHERE id=$1`, id, rawJSON(result))
		return err
	})
	return result, err
}

func emit(ctx context.Context, tx *sql.Tx, a Actor, event string, payload any) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO v2_outbox(id,operation_id,event_type,payload) VALUES($1,$2,$3,$4::jsonb) ON CONFLICT(operation_id,event_type) DO NOTHING`, newID(), a.OperationID, event, rawJSON(payload))
	return err
}

func command[T any](s *Service, action string, fn func(context.Context, *sql.Tx, Actor, T) (any, error)) echo.HandlerFunc {
	return func(c echo.Context) error {
		var input T
		decoder := json.NewDecoder(c.Request().Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return platform.HandleHTTPError(c, bad("ข้อมูลคำสั่งไม่ถูกต้อง"))
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return platform.HandleHTTPError(c, bad("ส่ง JSON ได้หนึ่งคำสั่งต่อครั้ง"))
		}
		ctx := c.Request().Context()
		result, err := s.execute(ctx, platform.CurrentUser(c), action, c.Request().Header.Get("Idempotency-Key"), input, func(tx *sql.Tx, a Actor) (any, error) { return fn(ctx, tx, a, input) })
		if err != nil {
			return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "ดำเนินการ V2 ไม่สำเร็จ", err))
		}
		return c.JSON(http.StatusOK, result)
	}
}

func (s *Service) listing(query string) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		branchID, err := branch(ctx, s.db, platform.CurrentUser(c), c.QueryParam("branch_id"))
		if err != nil {
			return platform.HandleHTTPError(c, err)
		}
		var data []byte
		if err = s.db.QueryRowContext(ctx, query, branchID).Scan(&data); err != nil {
			return platform.HandleHTTPError(c, platform.WrapError(500, "โหลดข้อมูล V2 ไม่สำเร็จ", err))
		}
		return c.JSONBlob(200, data)
	}
}
