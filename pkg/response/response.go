package response

import (
	"encoding/json"
	"github.com/gofiber/fiber/v3"
)

// Response 是通用的泛型 JSON 返回结构。
// 它为数据载荷使用了一个类型参数 T。
type Response[T any] struct {
	RequestID string `json:"request_id"` // 对应 X-Request-ID 请求头
	Code      int    `json:"code"`       // 自定义业务码
	Msg       string `json:"msg"`        // 业务码对应的消息
	Data      T      `json:"data"`       // 实际数据，具有特定的类型 T
}

const (
	// CodeSuccess 表示操作成功。
	CodeSuccess = 0
	// CodeError 表示通用业务错误。
	CodeError = 7
	// CodeInvalidParams 表示请求参数无效。
	CodeInvalidParams = 400
	// CodeNotFound 表示资源未找到。
	CodeNotFound = 404
	// CodeServerError 表示内部服务器错误。
	CodeServerError = 500
)

// JSON 发送一个使用泛型的结构化 JSON 返回。
func JSON[T any](c fiber.Ctx, httpStatus, code int, msg string, data T) error {
	return c.Status(httpStatus).JSON(&Response[T]{
		RequestID: c.GetRespHeader(fiber.HeaderXRequestID),
		Code:      code,
		Msg:       msg,
		Data:      data,
	})
}

// Success 发送一个标准的成功返回 (HTTP 200)。
// Go 的类型推断会根据传入的 data 参数自动确定 T 的类型。
func Success[T any](c fiber.Ctx, data T) error {
	return JSON(c, fiber.StatusOK, CodeSuccess, "ok", data)
}

// Created 发送一个表示资源创建成功的返回 (HTTP 201)。
func Created[T any](c fiber.Ctx, data T) error {
	return JSON(c, fiber.StatusCreated, CodeSuccess, "ok", data)
}

// NoContent 发送一个表示成功但无返回体的响应 (HTTP 204)。
func NoContent(c fiber.Ctx) error {
	// 204 No Content 响应不应该包含任何 body
	return c.Status(fiber.StatusNoContent).Send(nil)
}

// Fail 发送一个通用错误返回。
// 对于没有数据的返回，它使用 `any` 作为类型，`nil` 作为值，
// 这将被序列化为 `{"data": null}`。
func Fail(c fiber.Ctx, code int, msg string) error {
	httpStatus := fiber.StatusInternalServerError
	switch code {
	case CodeInvalidParams:
		httpStatus = fiber.StatusBadRequest
	case CodeNotFound:
		httpStatus = fiber.StatusNotFound
	}
	// 当没有数据时，我们显式地使用 `any` (interface{}) 和 nil。
	return JSON[any](c, httpStatus, code, msg, nil)
}

// FailWithData 发送一个包含额外数据的错误返回。
func FailWithData[T any](c fiber.Ctx, code int, msg string, data T) error {
	httpStatus := fiber.StatusInternalServerError
	switch code {
	case CodeInvalidParams:
		httpStatus = fiber.StatusBadRequest
	case CodeNotFound:
		httpStatus = fiber.StatusNotFound
	}
	return JSON(c, httpStatus, code, msg, data)
}

// structToMap 是一个辅助函数，使用 JSON 序列化和反序列化将任何结构体转换为 map。
// 注意：这是一种简单但性能非最优的方法，适用于大多数场景。
func structToMap(data interface{}) (map[string]interface{}, error) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(bytes, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// JSONFlat 发送一个扁平化的 JSON 响应。
// 它会将 data 对象的所有字段与基础响应字段合并在同一层级。
func JSONFlat(c fiber.Ctx, httpStatus, code int, msg string, data interface{}) error {
	base := map[string]interface{}{
		"request_id": c.GetRespHeader(fiber.HeaderXRequestID),
		"code":       code,
		"msg":        msg,
	}

	// 如果有业务数据，则将其字段合并到基础 map 中
	if data != nil {
		dataMap, err := structToMap(data)
		if err != nil {
			// 如果转换失败，返回一个内部错误，并在错误数据中说明原因
			return JSONFlat(c, fiber.StatusInternalServerError, CodeServerError, "服务器内部错误：无法序列化响应数据", nil)
		}

		for key, value := range dataMap {
			// 避免业务数据覆盖基础元数据字段
			if _, exists := base[key]; !exists {
				base[key] = value
			}
		}
	}

	return c.Status(httpStatus).JSON(base)
}

// SuccessFlat 发送一个扁平化的成功响应 (HTTP 200)。
func SuccessFlat(c fiber.Ctx, data interface{}) error {
	return JSONFlat(c, fiber.StatusOK, CodeSuccess, "ok", data)
}

// CreatedFlat 发送一个扁平化的资源创建成功响应 (HTTP 201)。
func CreatedFlat(c fiber.Ctx, data interface{}) error {
	return JSONFlat(c, fiber.StatusCreated, CodeSuccess, "ok", data)
}

// FailFlat 发送一个扁平化的错误响应。
func FailFlat(c fiber.Ctx, code int, msg string) error {
	httpStatus := fiber.StatusInternalServerError
	switch code {
	case CodeInvalidParams:
		httpStatus = fiber.StatusBadRequest
	case CodeNotFound:
		httpStatus = fiber.StatusNotFound
	}
	return JSONFlat(c, httpStatus, code, msg, nil)
}

// FailWithDataFlat 发送一个带数据的扁平化错误响应。
func FailWithDataFlat(c fiber.Ctx, code int, msg string, data interface{}) error {
	httpStatus := fiber.StatusInternalServerError
	switch code {
	case CodeInvalidParams:
		httpStatus = fiber.StatusBadRequest
	case CodeNotFound:
		httpStatus = fiber.StatusNotFound
	}
	return JSONFlat(c, httpStatus, code, msg, data)
}
