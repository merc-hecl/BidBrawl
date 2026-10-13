package hints

import (
	"errors"
	"fmt"
	"sort"
)

var ErrNotApplicable = errors.New("hint does not apply to this box")

var builtins = map[string]bool{
	"random_shape": true, "random_quality": true,
	"quality_count": true, "quality_average": true, "random_quality_price": true,
}

var qualityNames = map[string][2]string{
	"common": {"普通", "common"}, "uncommon": {"优良", "uncommon"},
	"rare": {"稀有", "rare"}, "epic": {"史诗", "epic"}, "legendary": {"传说", "legendary"},
}

func builtin(kind string, r Request) (Clue, error) {
	if len(r.Items) == 0 {
		return Clue{}, ErrNotApplicable
	}
	groups := map[string][]Item{}
	for _, item := range r.Items {
		groups[item.Quality] = append(groups[item.Quality], item)
	}
	qualities := []string{}
	for quality, items := range groups {
		// A price average must not describe a majority or the whole box.
		if kind != "quality_average" || len(items)*2 <= len(r.Items) {
			qualities = append(qualities, quality)
		}
	}
	sort.Strings(qualities)
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
	case "quality_count", "quality_average", "random_quality_price":
		if len(qualities) == 0 || kind == "random_quality_price" && len(r.Items) < 2 {
			return Clue{}, ErrNotApplicable
		}
		q := qualities[Random(len(qualities))]
		items := groups[q]
		fact.Quality = q
		name := qualityNames[q]
		switch kind {
		case "quality_count":
			count := len(items)
			fact.Count = &count
			zhTitle, enTitle = "指定品质数量", "Count for one quality"
			zh, en = fmt.Sprintf("箱内%s品质的物品共有 %d 件。", name[0], count), fmt.Sprintf("The box contains %d %s-quality items.", count, name[1])
		case "quality_average":
			var sum int64
			for _, item := range items {
				sum += item.Price
			}
			average := float64(sum) / float64(len(items))
			fact.AveragePrice = &average
			zhTitle, enTitle = "指定品质均价", "Average for one quality"
			zh, en = fmt.Sprintf("箱内%s品质物品的实际均价为 %.2f 金币。", name[0], average), fmt.Sprintf("The actual average price of %s-quality items in this box is %.2f coins.", name[1], average)
		case "random_quality_price":
			item := items[Random(len(items))]
			fact.ItemID, fact.Price = item.ID, &item.Price
			zhTitle, enTitle = "指定品质单件价格", "One item's price within a quality"
			zh, en = fmt.Sprintf("随机抽中的%s品质物品 %s，实际价格为 %d 金币。", name[0], item.ID, item.Price), fmt.Sprintf("Randomly selected %s-quality item %s has an actual price of %d coins.", name[1], item.ID, item.Price)
		}
	default:
		return Clue{}, fmt.Errorf("unknown hint builtin: %s", kind)
	}
	c := Clue{Text: Text{zhTitle, zh}, Translations: map[string]Text{"en": {enTitle, en}}, Fact: fact}
	return c, c.Validate()
}
