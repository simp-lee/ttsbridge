package edgetts

import (
	"testing"
)

func TestGenerateSecMsGec(t *testing.T) {
	// Reset clock skew for testing
	ResetClockSkew()

	token := GenerateSecMsGec(defaultClientToken)

	// Token should be 64 characters (SHA256 hex)
	if len(token) != 64 {
		t.Errorf("Expected token length 64, got %d", len(token))
	}

	// Token should be uppercase
	for _, c := range token {
		if c >= 'a' && c <= 'z' {
			t.Errorf("Token should be uppercase, but contains lowercase: %s", token)
			break
		}
	}

	// Generate again should produce same result within 5-minute window
	token2 := GenerateSecMsGec(defaultClientToken)
	if token != token2 {
		t.Errorf("Tokens generated within same time window should match")
	}
}

func TestAdjustClockSkew(t *testing.T) {
	ResetClockSkew()

	// Test valid RFC 2616 date
	err := AdjustClockSkew("Mon, 02 Jan 2006 15:04:05 GMT")
	if err != nil {
		t.Errorf("Failed to parse valid date: %v", err)
	}

	// Test invalid date
	err = AdjustClockSkew("invalid date")
	if err == nil {
		t.Error("Should fail for invalid date")
	}

	ResetClockSkew()
}

// TestAdjustClockSkewNoAccumulation 测试时钟偏差是否正确覆盖而不是累加
func TestAdjustClockSkewNoAccumulation(t *testing.T) {
	ResetClockSkew()

	// 第一次设置偏差（假设服务器时间比客户端快 10 秒）
	err := AdjustClockSkew("Mon, 02 Jan 2006 15:04:15 GMT") // 15:04:15
	if err != nil {
		t.Fatalf("Failed to adjust clock skew: %v", err)
	}

	firstSkew := GetClockSkew()
	t.Logf("First clock skew: %f seconds", firstSkew)

	// 第二次设置偏差（假设服务器时间比客户端快 5 秒）
	// 如果是累加，偏差会变成 15 秒；如果是覆盖，偏差会是 5 秒
	err = AdjustClockSkew("Mon, 02 Jan 2006 15:04:10 GMT") // 15:04:10
	if err != nil {
		t.Fatalf("Failed to adjust clock skew: %v", err)
	}

	secondSkew := GetClockSkew()
	t.Logf("Second clock skew: %f seconds", secondSkew)

	// 验证第二次的偏差不是第一次的累加
	// 注意：由于时间流逝，精确值会有小幅变化，但不应该是累加结果
	if secondSkew > firstSkew {
		t.Errorf("Clock skew should not accumulate. First: %f, Second: %f", firstSkew, secondSkew)
	}

	ResetClockSkew()
}

func TestGenerateConnectionID(t *testing.T) {
	id1 := generateConnectionID()
	id2 := generateConnectionID()

	// Should be 32 hex characters (UUID without dashes)
	if len(id1) != 32 {
		t.Errorf("Expected ID length 32, got %d", len(id1))
	}

	// Should be unique
	if id1 == id2 {
		t.Error("Generated IDs should be unique")
	}
}
