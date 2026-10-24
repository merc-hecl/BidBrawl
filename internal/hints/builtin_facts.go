package hints

import (
	"errors"
	"fmt"
	"sort"
)

var ErrNotApplicable = errors.New("hint does not apply to this box")

var builtins = map[string]bool{
	"random_shape": true, "random_quality": true,
	"quality_count":    true,
	"quality_presence": true, "quality_largest_area": true,
	"category_count": true, "category_quality_count": true, "region_quality_count": true,
	"quality_total_area": true, "quality_average_area": true,
}

var qualityNames = map[string][2]string{
	"common": {"普通", "common"}, "uncommon": {"优良", "uncommon"},
	"rare": {"稀有", "rare"}, "epic": {"史诗", "epic"}, "legendary": {"传说", "legendary"},
}

func builtin(kind string, r Request) (Clue, error) {
	if kind == "category_count" || kind == "category_quality_count" || kind == "region_quality_count" || kind == "quality_total_area" || kind == "quality_average_area" {
		return scopedBuiltin(kind, r)
	}
	if len(r.Items) == 0 && kind != "quality_presence" && kind != "quality_count" {
		return Clue{}, ErrNotApplicable
	}
	groups := map[string][]Item{}
	for _, item := range r.Items {
		groups[item.Quality] = append(groups[item.Quality], item)
	}
	qualities := []string{}
	for quality := range groups {
		qualities = append(qualities, quality)
	}
	sort.Strings(qualities)
	if kind == "quality_count" && len(qualities) == 0 {
		qualities = []string{"common", "uncommon", "rare", "epic", "legendary"}
	}
	fact := &Fact{Type: kind}
	var zhTitle, enTitle, zh, en string
	switch kind {
	case "random_shape", "random_quality":
		item := r.Items[Random(len(r.Items))]
		fact.ItemID = item.ID
		if kind == "random_shape" {
			fact.TopLeft, fact.BottomRight = &item.TopLeft, &item.BottomRight
			zhTitle, enTitle = "随机物品轮廓", "Random item shape"
			zh = fmt.Sprintf("物品 %s 占 %d × %d 格，坐标 (%d, %d) → (%d, %d)。", item.ID, item.BottomRight.X-item.TopLeft.X, item.BottomRight.Y-item.TopLeft.Y, item.TopLeft.X, item.TopLeft.Y, item.BottomRight.X, item.BottomRight.Y)
			en = fmt.Sprintf("Item %s occupies %d × %d cells, from (%d, %d) to (%d, %d).", item.ID, item.BottomRight.X-item.TopLeft.X, item.BottomRight.Y-item.TopLeft.Y, item.TopLeft.X, item.TopLeft.Y, item.BottomRight.X, item.BottomRight.Y)
		} else {
			fact.Quality = item.Quality
			zhTitle, enTitle = "随机物品品质", "Random item quality"
			zh, en = fmt.Sprintf("物品 %s 的品质为%s。", item.ID, qualityNames[item.Quality][0]), fmt.Sprintf("Item %s has %s quality.", item.ID, qualityNames[item.Quality][1])
		}
	case "quality_presence":
		all := []string{"common", "uncommon", "rare", "epic", "legendary"}
		fact.Quality = all[Random(len(all))]
		present := len(groups[fact.Quality]) > 0
		fact.Present = &present
		name := qualityNames[fact.Quality]
		zhTitle, enTitle = "指定品质是否存在", "Presence of one quality"
		if present {
			zh, en = fmt.Sprintf("箱内存在%s品质的物品。", name[0]), fmt.Sprintf("The box contains items of %s quality.", name[1])
		} else {
			zh, en = fmt.Sprintf("箱内没有%s品质的物品。", name[0]), fmt.Sprintf("The box contains no items of %s quality.", name[1])
		}
	case "quality_count", "quality_largest_area":
		if len(qualities) == 0 {
			return Clue{}, ErrNotApplicable
		}
		q := qualities[Random(len(qualities))]
		items := groups[q]
		fact.Quality = q
		name := qualityNames[q]
		switch kind {
		case "quality_largest_area":
			area := 0
			for _, item := range items {
				area = max(area, (item.BottomRight.X-item.TopLeft.X)*(item.BottomRight.Y-item.TopLeft.Y))
			}
			fact.Area = &area
			zhTitle, enTitle = "指定品质最大面积", "Largest area within one quality"
			zh = fmt.Sprintf("箱内%s品质物品中，最大的一件占 %d 格。", name[0], area)
			en = fmt.Sprintf("The largest %s-quality item occupies %d cells.", name[1], area)
		case "quality_count":
			count := len(items)
			fact.Count = &count
			zhTitle, enTitle = "指定品质数量", "Count for one quality"
			zh, en = fmt.Sprintf("箱内%s品质的物品共有 %d 件。", name[0], count), fmt.Sprintf("The box contains %d %s-quality items.", count, name[1])
		}
	default:
		return Clue{}, fmt.Errorf("unknown hint builtin: %s", kind)
	}
	c := Clue{Text: Text{zhTitle, zh}, Translations: map[string]Text{"en": {enTitle, en}}, Fact: fact}
	return c, c.Validate()
}
