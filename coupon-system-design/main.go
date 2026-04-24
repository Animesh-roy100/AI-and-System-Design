package main

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Priority Rules
type PriorityType string

const (
	PriorityPlatform   PriorityType = "PLATFORM"   // Platform's own offers (eg. SWIGGY10, SWIGGY50)
	PriorityBank       PriorityType = "BANK"       // Bank offers (eg. HDFC10)
	PriorityRestaurant PriorityType = "RESTAURANT" // Restaurant offers (eg. DOMINO10)
	PriorityPartner    PriorityType = "PARTNER"    // Partner offers (eg. Swiggy One, Zomato Gold)
)

// Multi-Use vs Single-Use Coupons
type UsageType string

const (
	UsageSingleUse UsageType = "SINGLE_USE" // One-time per user
	UsageMultiUse  UsageType = "MULTI_USE"  // N times per user
	UsageUnlimited UsageType = "UNLIMITED"  // Unlimited times per user (global)
	UsageTimeBound UsageType = "TIME_BOUND" // Limited time period, N times per user per day
)

// Discount Type
type DiscountType string

const (
	DiscountFlat         DiscountType = "FLAT"
	DiscountPercentage   DiscountType = "PERCENTAGE"
	DiscountFreeDelivery DiscountType = "FREE_DELIVERY"
	DiscountCashback     DiscountType = "CASHBACK"
)

// Coupon Status
type CouponStatus string

const (
	CouponStatusDraft   CouponStatus = "DRAFT"
	CouponStatusActive  CouponStatus = "ACTIVE"
	CouponStatusPaused  CouponStatus = "PAUSED"
	CouponStatusExpired CouponStatus = "EXPIRED"
)

// Redemption Status
type RedemptionStatus string

const (
	RedemptionRedeemed    RedemptionStatus = "REDEEMED"
	RedemptionInvalidated RedemptionStatus = "INVALIDATED"
)

// User Coupon Status
type UserCouponStatus string

const (
	UserCouponAvailable   UserCouponStatus = "AVAILABLE"
	UserCouponApplied     UserCouponStatus = "APPLIED"
	UserCouponRedeemed    UserCouponStatus = "REDEEMED"
	UserCouponExpired     UserCouponStatus = "EXPIRED"
	UserCouponInvalidated UserCouponStatus = "INVALIDATED"
)

// Rule Type
type RuleType string

const (
	RuleCity          RuleType = "CITY"
	RuleRestaurant    RuleType = "RESTAURANT"
	RuleCuisine       RuleType = "CUISINE"
	RulePaymentMethod RuleType = "PAYMENT_METHOD"
	RuleUserSegment   RuleType = "USER_SEGMENT"
	RuleFirstOrder    RuleType = "FIRST_ORDER"
	RuleNthOrder      RuleType = "NTH_ORDER"
	RuleDevice        RuleType = "DEVICE"
	RuleMinItems      RuleType = "MIN_ITEMS"
)

// Coupon represents the core offer definition.
type Coupon struct {
	ID                 uuid.UUID // primary key
	Code               string
	Title              string
	Description        string
	DiscountType       DiscountType
	DiscountValue      float64
	MaxDiscount        *float64 // nil for FLAT
	MinCartValue       float64
	StartTime          time.Time
	EndTime            time.Time
	Status             CouponStatus
	UsageType          UsageType
	PriorityType       PriorityType
	Priority           int
	MaxPerUser         *int
	MaxPerUserPerDay   *int
	MaxGlobal          *int
	CurrentGlobalUsage int
	IsVisible          bool
	IsStackable        bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CreatedBy          string
}

// CouponRule stores flexible eligibility constraints per coupon.
type CouponRule struct {
	ID          uuid.UUID // primary key
	CouponID    uuid.UUID
	RuleType    RuleType
	RuleValue   json.RawMessage // flexible payload
	IsInclusion bool            // true=whitelist, false=blacklist
	CreatedAt   time.Time
}

// UserCoupon tracks per-user coupon state (applied, redeemed, etc.).
type UserCoupon struct {
	ID                 uuid.UUID // primary key
	UserID             uuid.UUID
	CouponID           uuid.UUID
	Status             UserCouponStatus
	UsageCount         int
	ReservationOrderID *uuid.UUID // set when coupon is applied to a cart
	ReservedAt         *time.Time // set when applied
	ReedeemedAt        *time.Time // set when redeemed
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// CouponRedemptionLog is an immutable audit trail for every redemption attempt.
type CouponRedemtionLog struct {
	ID                 uuid.UUID // primary key
	UserID             uuid.UUID
	CouponID           uuid.UUID
	OrderID            uuid.UUID
	CouponCode         string
	CartValue          float64
	DiscountValue      float64
	FinalValue         float64
	Status             RedemptionStatus
	IdempotencyKey     *string
	InvalidationReason *string
	RedeemedAt         *time.Time
	InvalidatedAt      *time.Time
}
