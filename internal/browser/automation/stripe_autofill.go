package automation

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// CreditCardInfo 信用卡及账单详情定义
type CreditCardInfo struct {
	CardNumber   string `json:"cardNumber"`             // 卡号 (16位)
	ExpMonth     string `json:"expMonth"`               // 有效期月份 (如 08)
	ExpYear      string `json:"expYear"`                // 有效期年份 (如 28 或 2028)
	CVC          string `json:"cvc"`                    // 安全码 (3~4位)
	Name         string `json:"name"`                   // 持卡人英文全名
	Country      string `json:"country,omitempty"`      // 国家代码 (默认 US)
	PostalCode   string `json:"postalCode,omitempty"`   // 账单邮编 (如 90001)
	State        string `json:"state,omitempty"`        // 州/省 (如 CA)
	City         string `json:"city,omitempty"`         // 城市 (如 Los Angeles)
	AddressLine1 string `json:"addressLine1,omitempty"` // 详细地址 1
	AddressLine2 string `json:"addressLine2,omitempty"` // 详细地址 2
}

// AutoFillStripeCheckout 自动在 Stripe Checkout 页面中仿真填写卡片表单
func AutoFillStripeCheckout(paymentURL string, card CreditCardInfo, autoSubmit bool, cb ProgressCallback) error {
	report := func(msg string) {
		log.Printf("[StripeAutofill] %s", msg)
		if cb != nil {
			cb(msg)
		}
	}

	paymentURL = strings.TrimSpace(paymentURL)
	if !strings.HasPrefix(paymentURL, "https://checkout.stripe.com/") {
		return fmt.Errorf("无效的 Stripe 结账链接: %s", paymentURL)
	}

	// 规范化卡片字段
	cleanCardNumber := strings.ReplaceAll(strings.TrimSpace(card.CardNumber), " ", "")
	cleanMonth := strings.TrimSpace(card.ExpMonth)
	if len(cleanMonth) == 1 {
		cleanMonth = "0" + cleanMonth
	}
	cleanYear := strings.TrimSpace(card.ExpYear)
	if len(cleanYear) == 4 {
		cleanYear = cleanYear[2:] // 取后两位
	}
	expiryFormatted := cleanMonth + cleanYear
	cleanCVC := strings.TrimSpace(card.CVC)
	cleanName := strings.TrimSpace(card.Name)

	browserPath, err := FindBrowserPath()
	if err != nil {
		return err
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browserPath),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("window-size", "1100,850"),
		chromedp.Flag("headless", false),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, 90*time.Second)
	defer cancelTimeout()

	report(fmt.Sprintf("正在打开 Stripe 付款页面: %s", truncateURL(paymentURL)))
	if err := chromedp.Run(ctx, chromedp.Navigate(paymentURL)); err != nil {
		return fmt.Errorf("打开 Stripe 页面失败: %w", err)
	}

	// 等待卡号输入框渲染就绪 (最多等待 25 秒)
	report("等待 Stripe 结算表单元素加载...")
	cardInputSel := `#cardNumber, input[name="cardNumber"], input[autocomplete="cc-number"]`
	if err := chromedp.Run(ctx,
		chromedp.WaitVisible(cardInputSel, chromedp.ByQuery),
	); err != nil {
		return fmt.Errorf("Stripe 表单加载超时 (卡号输入框未呈现): %w", err)
	}
	time.Sleep(1 * time.Second)

	// 1. 填写卡号
	if cleanCardNumber != "" {
		report(fmt.Sprintf("正在仿真填写卡号 (%s****)...", cleanCardNumber[:4]))
		if err := simulateHumanType(ctx, cardInputSel, cleanCardNumber); err != nil {
			return fmt.Errorf("填入卡号失败: %w", err)
		}
		time.Sleep(300 * time.Millisecond)
	}

	// 2. 填写到期日 (MM / YY)
	if expiryFormatted != "" {
		report(fmt.Sprintf("正在填写有效期 (%s/%s)...", cleanMonth, cleanYear))
		expirySel := `#cardExpiry, input[name="cardExpiry"], input[autocomplete="cc-exp"]`
		_ = chromedp.Run(ctx, chromedp.Focus(expirySel, chromedp.ByQuery))
		if err := simulateHumanType(ctx, expirySel, expiryFormatted); err != nil {
			log.Printf("[StripeAutofill] 填入有效期重试: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
	}

	// 3. 填写 CVC
	if cleanCVC != "" {
		report("正在填写安全码 (CVC)...")
		cvcSel := `#cardCvc, input[name="cardCvc"], input[autocomplete="cc-csc"]`
		_ = chromedp.Run(ctx, chromedp.Focus(cvcSel, chromedp.ByQuery))
		if err := simulateHumanType(ctx, cvcSel, cleanCVC); err != nil {
			log.Printf("[StripeAutofill] 填入 CVC 异常: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
	}

	// 4. 填写持卡人全名
	if cleanName != "" {
		report(fmt.Sprintf("正在填写持卡人姓名 (%s)...", cleanName))
		nameSel := `#billingName, input[name="billingName"], input[autocomplete="cc-name"]`
		var hasNameEl bool
		_ = chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`!!document.querySelector(%q)`, nameSel), &hasNameEl))
		if hasNameEl {
			_ = simulateHumanType(ctx, nameSel, cleanName)
			time.Sleep(200 * time.Millisecond)
		}
	}

	// 5. 填写账单国家 (Select)
	if card.Country != "" {
		countryCode := strings.ToUpper(strings.TrimSpace(card.Country))
		report(fmt.Sprintf("正在选择账单国家 (%s)...", countryCode))
		selectCountryJS := fmt.Sprintf(`
(() => {
    const sel = document.querySelector('#billingCountry, select[name="billingCountry"]');
    if (sel) {
        sel.value = %q;
        sel.dispatchEvent(new Event('change', { bubbles: true }));
        return true;
    }
    return false;
})()
`, countryCode)
		_ = chromedp.Run(ctx, chromedp.Evaluate(selectCountryJS, nil))
		time.Sleep(500 * time.Millisecond)
	}

	// 6. 填写账单邮编
	if card.PostalCode != "" {
		report(fmt.Sprintf("正在填写邮编 (%s)...", card.PostalCode))
		postalSel := `#billingPostalCode, input[name="billingPostalCode"], input[autocomplete="billing postal-code"]`
		var hasPostalEl bool
		_ = chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`!!document.querySelector(%q)`, postalSel), &hasPostalEl))
		if hasPostalEl {
			_ = simulateHumanType(ctx, postalSel, card.PostalCode)
			time.Sleep(200 * time.Millisecond)
		}
	}

	// 7. 填写城市与详细地址 (若表单可见)
	if card.AddressLine1 != "" {
		addrSel := `#billingAddressLine1, input[name="billingAddressLine1"], input[autocomplete="billing address-line1"]`
		var hasAddrEl bool
		_ = chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`!!document.querySelector(%q)`, addrSel), &hasAddrEl))
		if hasAddrEl {
			report("正在填写详细地址...")
			_ = simulateHumanType(ctx, addrSel, card.AddressLine1)
		}
	}

	if card.City != "" {
		citySel := `#billingLocality, input[name="billingLocality"]`
		var hasCityEl bool
		_ = chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`!!document.querySelector(%q)`, citySel), &hasCityEl))
		if hasCityEl {
			_ = simulateHumanType(ctx, citySel, card.City)
		}
	}

	report("✅ 卡片及账单信息已全自动仿真填充完毕！")

	// 8. 提交判定
	if autoSubmit {
		report("正在自动提交订阅绑卡表单...")
		submitJS := `
(() => {
    const btn = document.querySelector('button[type="submit"], .SubmitButton');
    if (btn && !btn.disabled) {
        btn.click();
        return true;
    }
    return false;
})()
`
		_ = chromedp.Run(ctx, chromedp.Evaluate(submitJS, nil))
		report("已触发提交，正在等待银行 3D Secure 或扣款结果...")
		time.Sleep(6 * time.Second)
	} else {
		report("提示: 当前处于测试核验模式 (未自动点击扣款提交按钮)，表单已保留在浏览器中供确认。")
		// 为便于用户查看填写效果，等待 5 秒
		time.Sleep(5 * time.Second)
	}

	return nil
}
