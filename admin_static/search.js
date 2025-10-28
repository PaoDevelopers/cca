document.addEventListener("DOMContentLoaded", function() {
	const searchBar = document.getElementById("search-bar");
	const cards = document.querySelectorAll(".card");
	searchBar.addEventListener("input", function() {
		const query = searchBar.value.toLowerCase();
		cards.forEach(card => {
			const cardText = card.textContent.toLowerCase();
			if (cardText.includes(query)) {
				card.style.display = "";
			} else {
				card.style.display = "none";
			}
		});
	});
});
