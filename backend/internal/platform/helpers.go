package platform

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type ErrorResponse struct {
	Message string `json:"message"`
}

type AppError struct {
	Code       int
	Message    string
	WrappedErr error
}

func (e *AppError) Error() string {
	if e.WrappedErr == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.WrappedErr)
}

func NewError(code int, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// EnforceGhostWritePolicy protects operational write paths. Ghost quantity is
// changed only by the purchase-order, month-end and stock-claim services, which
// do not call this guard. Compatibility endpoints must call it before touching
// the DB so non-Superadmins still receive 403 while Superadmins receive an
// explanatory 400 response.
func EnforceGhostWritePolicy(user AuthUser, touchesGhost bool) error {
	if !touchesGhost {
		return nil
	}
	if user.RoleKey != "super_admin" {
		return NewError(http.StatusForbidden, "ไม่มีสิทธิ์จัดการสต๊อกผี")
	}
	return NewError(http.StatusBadRequest, "สต๊อกผีเปลี่ยนยอดได้เฉพาะใบสั่งซื้อเข้าและการสรุปสิ้นเดือน")
}

// EnforceGhostClaimPolicy guards the one operational path Ghost Stock is
// allowed down: a claim raised against stock on the shelf and sent to the
// supplier who supplied it. Unlike EnforceGhostWritePolicy this returns nil for
// a Superadmin rather than an explanation, because the claim is a real movement
// and not a compatibility stub — a defective Ghost unit has to be able to leave
// inventory, and receiving is the only way it got in.
//
// Every other role is refused outright: a claim is the only place central_admin
// and the tills could otherwise learn that Ghost Stock exists at all.
func EnforceGhostClaimPolicy(user AuthUser, touchesGhost bool) error {
	if !touchesGhost {
		return nil
	}
	if user.RoleKey != "super_admin" {
		return NewError(http.StatusForbidden, "ไม่มีสิทธิ์จัดการสต๊อกผี")
	}
	return nil
}

// WrapError gives a bare error a status and a message for the caller. A
// deliberate refusal underneath is kept as it stands: handlers commonly wrap
// whatever a service returns as "โหลด...ไม่สำเร็จ" with a 500, which would
// otherwise report a 403 branch-scope refusal as a server fault and hide the
// reason from the operator who could act on it.
func WrapError(code int, message string, err error) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) && appErr.Code < http.StatusInternalServerError {
		return appErr
	}
	return &AppError{Code: code, Message: message, WrappedErr: err}
}

func JSON(c echo.Context, code int, payload any) error {
	return c.JSON(code, payload)
}

func JSONMessage(c echo.Context, code int, message string) error {
	return c.JSON(code, map[string]any{"message": message})
}

func HandleHTTPError(c echo.Context, err error) error {
	var appErr *AppError
	if errors.As(err, &appErr) {
		if appErr.Code >= http.StatusInternalServerError {
			logServerError(c, err)
		}
		return c.JSON(appErr.Code, ErrorResponse{Message: thaiMessage(appErr.Message)})
	}
	logServerError(c, err)
	return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "เกิดข้อผิดพลาดภายในระบบ"})
}

// The customer-facing message for a 500 is deliberately vague, which left the
// actual cause nowhere at all — a failing endpoint could only be diagnosed by
// re-deriving it from the outside. The cause belongs in the server log, where
// the operator cannot see it and whoever is on call can.
//
// It is handed to the access log rather than printed, so a failed request is
// one JSON line carrying its cause alongside its path, status and user, instead
// of a plain-text line that has to be correlated with one by hand. Where no
// access log is running — a test, a CLI command — it falls back to printing.
func logServerError(c echo.Context, err error) {
	if err == nil {
		return
	}
	if _, ok := c.Get(ContextBranchAuditKey).(*BranchAudit); ok {
		c.Set(ContextServerErrorKey, err.Error())
		return
	}
	request := c.Request()
	log.Printf("500 %s %s: %v", request.Method, request.URL.Path, err)
}

func thaiMessage(message string) string {
	messages := map[string]string{
		"invalid credentials":                                                "อีเมลหรือรหัสผ่านไม่ถูกต้อง",
		"account is inactive":                                                "บัญชีนี้ถูกปิดใช้งาน",
		"role is inactive":                                                   "บทบาทนี้ถูกปิดใช้งาน",
		"invalid request body":                                               "ข้อมูลที่ส่งมาไม่ถูกต้อง",
		"branch scope mismatch":                                              "ไม่มีสิทธิ์เข้าถึงสาขานี้",
		"branch context is required":                                         "กรุณาระบุสาขา",
		"one side of the transfer must match your branch":                    "ต้นทางหรือปลายทางต้องเป็นสาขาของคุณ",
		"source and destination branch must differ":                          "สาขาต้นทางและปลายทางต้องไม่ซ้ำกัน",
		"transfer items are required":                                        "กรุณาเพิ่มสินค้าที่ต้องการโอน",
		"only source branch can dispatch":                                    "เฉพาะสาขาต้นทางที่ยืนยันการส่งได้",
		"transfer is not in requested state":                                 "รายการโอนไม่ได้อยู่ในสถานะรอส่ง",
		"only destination branch can receive":                                "เฉพาะสาขาปลายทางที่รับสินค้าได้",
		"transfer is not in transit":                                         "รายการโอนไม่ได้อยู่ระหว่างขนส่ง",
		"insufficient real stock for transfer dispatch":                      "สต๊อกจริงไม่เพียงพอสำหรับการโอน",
		"insufficient ghost stock for transfer dispatch":                     "สต๊อกผีไม่เพียงพอสำหรับการโอน",
		"invoice_id is required":                                             "กรุณาเลือกใบขาย",
		"start_date must be in YYYY-MM-DD format":                            "วันที่เริ่มต้นไม่ถูกต้อง",
		"invoice not found":                                                  "ไม่พบใบขาย",
		"payment_type must be cash or bank_transfer":                         "กรุณาเลือกชำระด้วยเงินสดหรือเงินโอน",
		"amount must be greater than zero":                                   "ยอดชำระต้องมากกว่าศูนย์",
		"invoice branch mismatch":                                            "ใบขายไม่ได้อยู่ในสาขาที่เลือก",
		"invoice is already settled":                                         "ใบขายนี้ชำระแล้ว",
		"password is required":                                               "กรุณากรอกรหัสผ่าน",
		"next_number must be greater than zero":                              "เลขถัดไปต้องมากกว่าศูนย์",
		"sequence not found":                                                 "ไม่พบชุดเลขที่เอกสาร",
		"prefix is required":                                                 "กรุณากรอกคำนำหน้าเลขที่เอกสาร",
		"sequence is locked; unlock it before editing prefix or next number": "ชุดเลขที่ถูกล็อก กรุณาปลดล็อกก่อนแก้ไข",
		"sequence is locked; prefix and next number cannot be edited":        "ชุดเลขที่ถูกล็อกและไม่สามารถแก้ไขได้",
		"branch scope is required":                                           "บัญชีนี้ยังไม่ได้กำหนดสาขา",
		"date must be YYYY-MM-DD":                                            "รูปแบบวันที่ไม่ถูกต้อง",
		"invalid rebalance request":                                          "ข้อมูลการย้ายสต๊อกไม่ถูกต้อง",
		"branch_id is required":                                              "กรุณาเลือกสาขา",
		"quantity_delta must not be zero":                                    "จำนวนที่ปรับต้องไม่เป็นศูนย์",
		"invalid stock bucket":                                               "ประเภทสต๊อกไม่ถูกต้อง",
		"quantities must not be negative":                                    "จำนวนรับเข้าต้องไม่ติดลบ",
		"at least one of real_quantity or ghost_quantity is required":        "กรุณาระบุจำนวนสต๊อกจริงหรือสต๊อกผีอย่างน้อยหนึ่งรายการ",
		"product not found":                                                  "ไม่พบสินค้า",
		"insufficient real stock":                                            "สต๊อกจริงไม่เพียงพอ",
		"insufficient ghost stock":                                           "สต๊อกผีไม่เพียงพอ",
		"real stock would become negative":                                   "สต๊อกจริงไม่สามารถติดลบได้",
		"ghost stock would become negative":                                  "สต๊อกผีไม่สามารถติดลบได้",
		"invoice is already paid":                                            "ใบขายนี้ชำระแล้ว",
		"only branch POS can collect direct cash or bank payments":           "เฉพาะพนักงานขายหน้าร้านที่รับชำระเงินสดหรือเงินโอนได้",
		"draft quotation not found":                                          "ไม่พบใบเสนอราคาฉบับร่าง",
		"at least one line is required":                                      "กรุณาเพิ่มสินค้าอย่างน้อยหนึ่งรายการ",
		"quantity must be greater than zero":                                 "จำนวนสินค้าต้องมากกว่าศูนย์",
		"stock bucket must be real or ghost":                                 "กรุณาเลือกสต๊อกจริงหรือสต๊อกผี",
		"price_tier must be cash or retail":                                  "ระดับราคาไม่ถูกต้อง",
		"alias does not match the selected product or branch":                "ชื่อ alias ไม่ตรงกับสินค้าหรือสาขาที่เลือก",
		"price override is not allowed":                                      "ไม่มีสิทธิ์กำหนดราคาพิเศษ",
		"failed to load user":                                                "โหลดข้อมูลผู้ใช้ไม่สำเร็จ",
		"failed to update login timestamp":                                   "บันทึกเวลาเข้าสู่ระบบไม่สำเร็จ",
		"failed to sign token":                                               "สร้างข้อมูลเข้าสู่ระบบไม่สำเร็จ",
		"failed to load permissions":                                         "โหลดสิทธิ์ไม่สำเร็จ",
		"failed to load branches":                                            "โหลดสาขาไม่สำเร็จ",
		"failed to load sequences":                                           "โหลดเลขที่เอกสารไม่สำเร็จ",
		"failed to load users":                                               "โหลดผู้ใช้ไม่สำเร็จ",
		"failed to load roles":                                               "โหลดบทบาทไม่สำเร็จ",
		"failed to load inventory":                                           "โหลดสต๊อกไม่สำเร็จ",
		"failed to load quotations":                                          "โหลดใบเสนอราคาไม่สำเร็จ",
		"failed to load invoices":                                            "โหลดใบขายไม่สำเร็จ",
		"failed to load outstanding invoices":                                "โหลดใบขายค้างชำระไม่สำเร็จ",
		"failed to load transfers":                                           "โหลดรายการโอนไม่สำเร็จ",
		"failed to load audit logs":                                          "โหลดประวัติการทำงานไม่สำเร็จ",
		"failed to load tax report":                                          "โหลดรายงานภาษีไม่สำเร็จ",
		"failed to load profit/loss report":                                  "โหลดรายงานกำไรขาดทุนไม่สำเร็จ",
		"failed to load marketplace providers":                               "โหลดผู้ให้บริการตลาดออนไลน์ไม่สำเร็จ",
		"failed to load marketplace orders":                                  "โหลดคำสั่งซื้อจากตลาดออนไลน์ไม่สำเร็จ",
		"user not found":                                                     "ไม่พบผู้ใช้",
		"report definition version is not supported":                         "รูปแบบรายงานเป็นเวอร์ชันที่ระบบยังไม่รองรับ",
		"report dataset is not allowed":                                      "ชุดข้อมูลรายงานนี้ไม่ได้รับอนุญาต",
		"report columns are required":                                        "กรุณาเลือกคอลัมน์อย่างน้อยหนึ่งรายการ",
		"report has too many columns":                                        "รายงานมีคอลัมน์เกิน 50 รายการ",
		"report page size is not allowed":                                    "จำนวนแถวต่อหน้าไม่ถูกต้อง",
		"report field sidebar width is not allowed":                          "ความกว้างแถบ All Fields ต้องอยู่ระหว่าง 220 ถึง 560 พิกเซล",
		"report field is not allowed":                                        "ฟิลด์รายงานนี้ไม่ได้รับอนุญาต",
		"report column label is too long":                                    "ชื่อหัวคอลัมน์ยาวเกินไป",
		"report column format is not allowed":                                "รูปแบบคอลัมน์นี้ไม่ได้รับอนุญาต",
		"report aggregate is not allowed":                                    "การคำนวณสรุปนี้ใช้กับฟิลด์ไม่ได้",
		"report column cannot be grouped and aggregated together":            "คอลัมน์เดียวกันไม่สามารถ Group และ Aggregate พร้อมกันได้",
		"all non-aggregate report columns must be grouped":                   "เมื่อใช้ Aggregate ต้อง Group คอลัมน์อื่นทั้งหมด",
		"report has too many filters":                                        "รายงานมีเงื่อนไขกรองเกิน 25 รายการ",
		"report has too many sort fields":                                    "รายงานเรียงข้อมูลได้สูงสุด 5 ระดับ",
		"report sort field is not allowed":                                   "ไม่สามารถเรียงด้วยฟิลด์นี้ได้",
		"report sort direction is not allowed":                               "ทิศทางการเรียงไม่ถูกต้อง",
		"report time field is not allowed":                                   "ฟิลด์เวลานี้ใช้สร้าง timeline ไม่ได้",
		"report timeline bucket is not allowed":                              "ช่วงของ timeline ไม่ถูกต้อง",
		"report relation depth is too deep":                                  "ความสัมพันธ์ของข้อมูลลึกเกิน 4 ระดับ",
		"report filter logic is not allowed":                                 "ตรรกะของเงื่อนไขกรองไม่ถูกต้อง",
		"report filter nesting is too deep":                                  "กลุ่มเงื่อนไขซ้อนกันลึกเกินไป",
		"report filter field is not allowed":                                 "ฟิลด์ที่ใช้กรองไม่ได้รับอนุญาต",
		"report filter operator is not allowed":                              "ตัวดำเนินการกรองใช้กับฟิลด์นี้ไม่ได้",
		"report filter value is required":                                    "กรุณาระบุค่าที่ต้องการกรอง",
		"report time range is invalid":                                       "ช่วงเวลาเริ่มต้นต้องไม่อยู่หลังเวลาสิ้นสุด",
		"report query is too broad or took too long":                         "คำค้นกว้างหรือซับซ้อนเกินไป กรุณาเพิ่มตัวกรองแล้วลองใหม่",
		"report definition is required":                                      "กรุณาระบุรูปแบบรายงาน",
		"report name is required":                                            "กรุณาตั้งชื่อรายงาน",
		"report name already exists":                                         "มีชื่อรายงานนี้อยู่แล้วในบัญชีของคุณ",
		"report not found":                                                   "ไม่พบรายงานนี้หรือคุณไม่ใช่เจ้าของ",
		"report id is invalid":                                               "รหัสรายงานไม่ถูกต้อง",
	}
	if translated, ok := messages[message]; ok {
		return translated
	}
	return message
}

func MustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func Round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func GenerateReadableCode(prefix string) string {
	compactID := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	return fmt.Sprintf("%s-%s-%s", strings.ToUpper(strings.TrimSpace(prefix)), time.Now().Format("20060102"), compactID[:6])
}

func NullString(value string) sql.NullString {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: trimmed, Valid: true}
}

func NullFloat64(value *float64) sql.NullFloat64 {
	if value == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: Round2(*value), Valid: true}
}

func NullUUID(value *string) sql.NullString {
	if value == nil || strings.TrimSpace(*value) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func MustUUID() string {
	return uuid.NewString()
}

func Timestamp() time.Time {
	return time.Now().UTC()
}
