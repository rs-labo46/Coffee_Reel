package entity

type CategoryCode string

const (
	CategoryBrewing   CategoryCode = "brewing"
	CategoryRoasting  CategoryCode = "roasting"
	CategoryLatteArt  CategoryCode = "latte_art"
	CategoryBeans     CategoryCode = "beans"
	CategoryEquipment CategoryCode = "equipment"
)

func (c CategoryCode) IsValid() bool {
	switch c {
	case CategoryBrewing, CategoryRoasting, CategoryLatteArt, CategoryBeans, CategoryEquipment:
		return true
	default:
		return false
	}
}
