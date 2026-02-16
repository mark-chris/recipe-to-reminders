package ingredients

import "strings"

var categoryKeywords = map[string][]string{
	"produce": {
		"carrot", "onion", "garlic", "potato", "tomato", "lettuce", "spinach",
		"celery", "pepper", "broccoli", "cauliflower", "zucchini", "squash",
		"cucumber", "mushroom", "avocado", "lemon", "lime", "orange", "apple",
		"banana", "berry", "strawberry", "blueberry", "grape", "mango",
		"pineapple", "peach", "pear", "ginger", "jalapeño", "shallot",
		"scallion", "green onion", "corn", "cabbage", "kale", "arugula",
		"beet", "radish", "turnip", "parsnip", "sweet potato", "eggplant",
	},
	"meat": {
		"beef", "chicken", "pork", "lamb", "turkey", "bacon", "sausage",
		"steak", "ground", "ham", "veal", "duck", "shrimp", "salmon",
		"tuna", "cod", "tilapia", "crab", "lobster", "scallop", "fish",
		"prosciutto", "pancetta", "anchov",
	},
	"dairy": {
		"milk", "butter", "cheese", "cream", "yogurt", "sour cream",
		"buttermilk", "parmesan", "mozzarella", "cheddar", "ricotta",
		"mascarpone", "whipping cream", "half-and-half", "egg",
	},
	"pantry": {
		"flour", "sugar", "oil", "vinegar", "soy sauce", "rice",
		"pasta", "noodle", "bread", "broth", "stock", "honey",
		"maple syrup", "mustard", "ketchup", "mayonnaise", "tomato paste",
		"tomato sauce", "coconut milk", "beans", "lentil", "chickpea",
		"peanut butter", "jam", "cornstarch", "baking powder", "baking soda",
		"yeast", "chocolate", "cocoa", "vanilla", "oat", "cereal",
		"cracker", "tortilla", "spaghetti", "penne",
	},
	"spices": {
		"salt", "pepper", "cumin", "paprika", "cinnamon", "nutmeg",
		"oregano", "basil", "thyme", "rosemary", "parsley", "cilantro",
		"dill", "bay leaf", "chili powder", "curry", "turmeric",
		"cayenne", "garlic powder", "onion powder", "clove",
		"cardamom", "coriander", "fennel seed", "red pepper flake",
		"italian seasoning", "sage", "tarragon", "mint",
	},
	"frozen": {
		"frozen",
	},
	"bakery": {
		"bread", "bun", "roll", "bagel", "croissant", "pita",
		"tortilla", "naan", "baguette",
	},
}

// Categorize assigns a grocery aisle category to an ingredient name.
func Categorize(name string) string {
	lower := strings.ToLower(name)

	// Check each category for keyword matches
	// Order matters: check more specific categories first
	for _, cat := range []string{"spices", "meat", "dairy", "produce", "frozen", "bakery", "pantry"} {
		for _, keyword := range categoryKeywords[cat] {
			if strings.Contains(lower, keyword) {
				return cat
			}
		}
	}
	return "other"
}
