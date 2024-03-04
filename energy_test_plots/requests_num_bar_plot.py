import matplotlib.pyplot as plt
import matplotlib.patches as mpatches


def plot_bar():
    numbers = [1615, 2838, 2356, 3475]
    labels = ['Euler', 'Euler energy', 'Classification', 'Classification energy']

    ranges_euler_light = [(0, 1008, 'steelblue'), (1008, 2213, 'lightgreen'), (2213, 2838, 'orange')]
    ranges_ml_light = [(0, 1403, 'steelblue'), (1403, 2303, 'lightgreen'), (2303, 3475, 'orange')]

    plt.figure(figsize=(11.69,8.27))

    blue_legend = mpatches.Patch(color='steelblue', label='Baseline requests')
    green_legend = mpatches.Patch(color='lightgreen', label='Energy-aware Variant requests')
    orange_legend = mpatches.Patch(color='orange', label='ffloaded requests')

    for i, (label, number) in enumerate(zip(labels, numbers)):
        if i == 1:
            for start, end, color in ranges_euler_light:
                plt.bar(label, min(end, number) - max(start, 0), bottom=max(start, 0), color=color)
        elif i == 3:
            for start, end, color in ranges_ml_light:
                plt.bar(label, min(end, number) - max(start, 0), bottom=max(start, 0), color=color)
        else:
            plt.bar(label, number, color='steelblue')

    plt.xlabel('Executed functions during evaluation', fontsize=18)
    plt.ylabel('Completed requests', fontsize=18)
    plt.legend(handles=[blue_legend, green_legend, orange_legend], loc=2,
               fontsize=16)

    plt.yticks(range(0, max(numbers) + 500, 250))

    plt.tick_params(axis='both', labelsize=18)
    #plt.show()
    plt.savefig("bar.pdf", format="PDF")


def main():
    plot_bar()


if __name__ == "__main__":
    main()
