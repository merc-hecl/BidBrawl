package hints

import (
	"errors"
	"math"
)

// Fact contains only approved public fields, never whole-box prices or bands.
type Fact struct {
	Category     string   `json:"category,omitempty"`
	Region       string   `json:"region,omitempty"`
	AverageArea  *float64 `json:"average_area,omitempty"`
	Type         string   `json:"type"`
	ItemID       string   `json:"item_id,omitempty"`
	Quality      string   `json:"quality,omitempty"`
	Count        *int     `json:"count,omitempty"`
	AveragePrice *float64 `json:"average_price,omitempty"`
	Price        *int64   `json:"price,omitempty"`
	TopLeft      *Point   `json:"top_left,omitempty"`
	BottomRight  *Point   `json:"bottom_right,omitempty"`
	Present      *bool    `json:"present,omitempty"`
	Area         *int     `json:"area,omitempty"`
}

func (f Fact) IsPrice() bool { return f.Type == "quality_average" || f.Type == "random_quality_price" }

func (f Fact) Validate() error {
	fields := 0
	if f.Category != "" {
		fields++
		if !validID.MatchString(f.Category) || f.Category == "easter_egg" {
			return errors.New("invalid hint category")
		}
	}
	if f.Region != "" {
		fields++
		if f.Region != "northwest" && f.Region != "northeast" && f.Region != "southwest" && f.Region != "southeast" {
			return errors.New("invalid hint region")
		}
	}
	if f.AverageArea != nil {
		fields++
		if math.IsNaN(*f.AverageArea) || math.IsInf(*f.AverageArea, 0) || *f.AverageArea < 1 || *f.AverageArea > 16384 {
			return errors.New("invalid hint mean area")
		}
	}
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
	if f.Present != nil {
		fields++
	}
	if f.Area != nil {
		fields++
		if *f.Area < 1 || *f.Area > 128*128 {
			return errors.New("invalid hint area")
		}
	}
	valid := false
	switch f.Type {
	case "category_count":
		valid = fields == 2 && f.Category != "" && f.Count != nil
	case "category_quality_count":
		valid = fields == 3 && f.Category != "" && f.Quality != "" && f.Count != nil
	case "region_quality_count":
		valid = fields == 3 && f.Region != "" && f.Quality != "" && f.Count != nil
	case "quality_total_area":
		valid = fields == 2 && f.Quality != "" && f.Area != nil
	case "quality_average_area":
		valid = fields == 2 && f.Quality != "" && f.AverageArea != nil
	case "quality_presence":
		valid = fields == 2 && f.Quality != "" && f.Present != nil
	case "quality_largest_area":
		valid = fields == 2 && f.Quality != "" && f.Area != nil
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
