document.addEventListener("DOMContentLoaded", function () {
	const searchBar = document.getElementById("search-bar");
	const cards = document.querySelectorAll(".card");

	const getVisibleText = card => {
		const clone = card.cloneNode(true);
		clone.querySelectorAll("details").forEach(details => details.remove());
		clone.querySelectorAll("form").forEach(form => form.remove());
		return clone.textContent.toLowerCase();
	};

	const cardVisibleText = new Map();
	cards.forEach(card => {
		cardVisibleText.set(card, getVisibleText(card));
	});

	searchBar.addEventListener("input", function () {
		const query = searchBar.value.toLowerCase();
		cards.forEach(card => {
			const cardText = cardVisibleText.get(card);
			card.style.display = cardText.includes(query) ? "" : "none";
		});
	});
});
