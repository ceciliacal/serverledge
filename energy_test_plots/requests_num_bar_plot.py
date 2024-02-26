import matplotlib.pyplot as plt

def plot_bar():

    numbers = [2838, 1615, 3475, 2356]
    #euler -> 1008 (sta al 40%),  2213 fa offloading!
    # ml -> 1403 (sta al 40%) , invece dalla 2303 fa offloading!
    labels = ['Euler light', 'Euler baseline', 'Machine Learning light', 'Machine Learning baseline']

    ranges_euler_light = [(0, 1008, 'steelblue'), (1008, 2213, 'lightgreen'), (2213, 2838, 'sandybrown')]
    ranges_ml_light = [(0, 1403, 'steelblue'), (1403, 2303, 'lightgreen'), (2303, 3475, 'sandybrown')]

    plt.figure(figsize=(10, 6))

    for i, (label, number) in enumerate(zip(labels, numbers)):
        if i == 0:
            for start, end, color in ranges_euler_light:
                plt.bar(label, min(end, number) - max(start, 0), bottom=max(start, 0), color=color)
        elif i == 2:
            for start, end, color in ranges_ml_light:
                plt.bar(label, min(end, number) - max(start, 0), bottom=max(start, 0), color=color)
        else:
            plt.bar(label, number, color='steelblue')


    plt.xlabel('Executed functions during evaluation')
    plt.ylabel('Requests number')
    plt.title('Requests number in different functions')

    plt.yticks(range(0, max(numbers) + 500, 250))

    plt.show()
    plt.savefig("requests_num_bar.pdf", format="PDF")


def main():
    plot_bar()

if __name__ == "__main__":
    main()