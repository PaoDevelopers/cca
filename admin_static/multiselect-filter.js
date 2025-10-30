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

		input.addEventListener("input", () => {
			const query = input.value.trim().toLowerCase();
			Array.from(select.options).forEach(option => {
				const matches = option.textContent.toLowerCase().includes(query);
				option.hidden = !matches;
			});
		});

		const display = document.getElementById(`${targetId}-display`);
		if (display) {
			const updateSelectedList = () => {
				const selected = Array.from(select.selectedOptions).map(opt => opt.textContent.trim());
				display.innerHTML = "";

				if (selected.length) {
					const ul = document.createElement("ul");
					selected.forEach(text => {
						const li = document.createElement("li");
						li.textContent = text;
						ul.appendChild(li);
					});
					display.appendChild(ul);
				} else {
					const none = document.createElement("p");
					none.textContent = "None selected.";
					display.appendChild(none);
				}
			};

			updateSelectedList();
			select.addEventListener("change", updateSelectedList);
		}
	});
});
