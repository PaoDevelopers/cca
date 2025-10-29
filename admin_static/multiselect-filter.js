document.addEventListener("DOMContentLoaded", () => {
	document.querySelectorAll(".multiselect-filter").forEach(input => {
		const targetId = input.getAttribute("data-filter-target");
		if (!targetId) {
			return;
		}

		const select = document.getElementById(targetId);
		if (!select) {
			return;
		}

		const options = Array.from(select.options);

		input.addEventListener("input", () => {
			const query = input.value.trim().toLowerCase();
			options.forEach(option => {
				const matches = option.textContent.toLowerCase().includes(query);
				option.hidden = !matches;
			});
		});
	});
});
