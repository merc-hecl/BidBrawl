package hints

import (
	"fmt"
	"sort"
)

// A scoped statistic names its entire population without disclosing identities,
// other groups, a whole-box count, or any price.
func scopedBuiltin(kind string, r Request) (Clue, error) {
	f := &Fact{Type: kind}
	pool := []Item{}
	var zhTitle, enTitle, zh, en string
	switch kind {
	case "category_count", "category_quality_count":
		categories := map[string]bool{}
		for _, i := range r.Items {
			if i.Category != "" && i.Category != "easter_egg" {
				categories[i.Category] = true
			}
		}
		ids := []string{}
		for id := range categories {
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			return Clue{}, ErrNotApplicable
		}
		sort.Strings(ids)
		f.Category = ids[Random(len(ids))]
		name := r.Categories[f.Category]
		if name == "" {
			name = f.Category
		}
		if kind == "category_quality_count" {
			qualities := []string{"common", "uncommon", "rare", "epic", "legendary"}
			f.Quality = qualities[Random(len(qualities))]
		}
		for _, i := range r.Items {
			if i.Category == f.Category && (f.Quality == "" || i.Quality == f.Quality) {
				pool = append(pool, i)
			}
		}
		zhTitle, enTitle = "指定品类件数", "Category count"
		zh, en = fmt.Sprintf("%s类物品共 %d 件。", name, len(pool)), fmt.Sprintf("The %s category contains %d items.", f.Category, len(pool))
		if f.Quality != "" {
			zhTitle, enTitle = "品类与品质交叉清点", "Category and quality count"
			zh, en = fmt.Sprintf("%s类中，%s品质物品共 %d 件。", name, qualityNames[f.Quality][0], len(pool)), fmt.Sprintf("The %s category contains %d %s-quality items.", f.Category, len(pool), qualityNames[f.Quality][1])
		}
		n := len(pool)
		f.Count = &n
	case "region_quality_count":
		regions := []string{"northwest", "northeast", "southwest", "southeast"}
		zhNames := []string{"左上", "右上", "左下", "右下"}
		n := Random(4)
		f.Region = regions[n]
		qualities := []string{"common", "uncommon", "rare", "epic", "legendary"}
		f.Quality = qualities[Random(len(qualities))]
		for _, i := range r.Items {
			east, south := i.TopLeft.X >= r.BoxWidth/2, i.TopLeft.Y >= r.BoxHeight/2
			if east == (n%2 == 1) && south == (n >= 2) && i.Quality == f.Quality {
				pool = append(pool, i)
			}
		}
		count := len(pool)
		f.Count = &count
		zhTitle, enTitle = "分区品质清点", "Regional quality count"
		zh = fmt.Sprintf("按左上角归区，%s象限内%s品质物品共 %d 件；中线为 x=%d、y=%d。", zhNames[n], qualityNames[f.Quality][0], count, r.BoxWidth/2, r.BoxHeight/2)
		en = fmt.Sprintf("By top-left corner, the %s quadrant contains %d %s-quality items; dividing lines are x=%d and y=%d.", f.Region, count, qualityNames[f.Quality][1], r.BoxWidth/2, r.BoxHeight/2)
	case "quality_total_area", "quality_average_area":
		groups := map[string][]Item{}
		for _, i := range r.Items {
			groups[i.Quality] = append(groups[i.Quality], i)
		}
		qualities := []string{}
		for q := range groups {
			qualities = append(qualities, q)
		}
		if len(qualities) == 0 {
			return Clue{}, ErrNotApplicable
		}
		sort.Strings(qualities)
		f.Quality = qualities[Random(len(qualities))]
		area := 0
		for _, i := range groups[f.Quality] {
			area += (i.BottomRight.X - i.TopLeft.X) * (i.BottomRight.Y - i.TopLeft.Y)
		}
		if kind == "quality_total_area" {
			f.Area = &area
			zhTitle, enTitle = "指定品质总占格", "Total area of one quality"
			zh, en = fmt.Sprintf("%s品质物品合计占 %d 格。", qualityNames[f.Quality][0], area), fmt.Sprintf("%s-quality items occupy %d cells in total.", qualityNames[f.Quality][1], area)
		} else {
			avg := float64(area) / float64(len(groups[f.Quality]))
			f.AverageArea = &avg
			zhTitle, enTitle = "指定品质平均占格", "Mean area of one quality"
			zh, en = fmt.Sprintf("%s品质物品平均占 %.2f 格。", qualityNames[f.Quality][0], avg), fmt.Sprintf("%s-quality items occupy %.2f cells on average.", qualityNames[f.Quality][1], avg)
		}
	default:
		return Clue{}, ErrNotApplicable
	}
	c := Clue{Text: Text{zhTitle, zh}, Translations: map[string]Text{"en": {enTitle, en}}, Fact: f}
	return c, c.Validate()
}
