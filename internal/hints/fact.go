package hints

import (
	"errors"
	"math"
)

// Fact contains only approved public fields, never whole-box prices or bands.
type Fact struct {
	Type         string   `json:"type"`
	ItemID       string   `json:"item_id,omitempty"`
	Quality      string   `json:"quality,omitempty"`
	Count        *int     `json:"count,omitempty"`
	AveragePrice *float64 `json:"average_price,omitempty"`
	Price        *int64   `json:"price,omitempty"`
	TopLeft      *Point   `json:"top_left,omitempty"`
	BottomRight  *Point   `json:"bottom_right,omitempty"`
}

func (f Fact) IsPrice() bool { return f.Type == "quality_average" || f.Type == "random_quality_price" }

func (f Fact) Validate() error {
	fields := 0
	if f.ItemID != "" {
		fields++
		if !validText(f.ItemID, 64) {
			return errors.New("invalid hint item ID")
		}
	}
	if f.Quality != "" {
		fields++
		if _, ok := qualityNames[f.Quality]; !ok {
			return errors.New("unknown hint quality")
		}
	}
	if f.Count != nil {
		fields++
		if *f.Count < 0 || *f.Count > 4096 {
			return errors.New("invalid hint count")
		}
	}
	if f.AveragePrice != nil {
		fields++
		if math.IsNaN(*f.AveragePrice) || math.IsInf(*f.AveragePrice, 0) || *f.AveragePrice <= 0 {
			return errors.New("invalid hint average")
		}
	}
	if f.Price != nil {
		fields++
		if *f.Price <= 0 {
			return errors.New("invalid hint price")
		}
	}
	if f.TopLeft != nil {
		fields++
	}
	if f.BottomRight != nil {
		fields++
	}
	valid := false
	switch f.Type {
	case "quality_count":
		valid = fields == 2 && f.Quality != "" && f.Count != nil
	case "quality_average":
		valid = fields == 2 && f.Quality != "" && f.AveragePrice != nil
	case "random_quality":
		valid = fields == 2 && f.ItemID != "" && f.Quality != ""
	case "random_quality_price":
		valid = fields == 3 && f.ItemID != "" && f.Quality != "" && f.Price != nil
	case "random_shape":
		valid = fields == 3 && f.ItemID != "" && f.TopLeft != nil && f.BottomRight != nil
		if valid {
			valid = f.TopLeft.X >= 0 && f.TopLeft.Y >= 0 && f.BottomRight.X > f.TopLeft.X && f.BottomRight.Y > f.TopLeft.Y && f.BottomRight.X <= 128 && f.BottomRight.Y <= 128
		}
	}
	if !valid {
		return errors.New("invalid or unsupported public hint fact")
	}
	return nil
}
