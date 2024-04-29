from tasks.libs.common.color import color_message


def yes_no_question(input_message, color="white", default=None):
    choice = None
    valid_answers = {'yes': True, 'y': True, 'no': False, 'n': False, '': default}

    if default is None:
        default_answer_prompt = "[y/n]"
    elif default:
        default_answer_prompt = "[Y/n]"
    else:
        default_answer_prompt = "[y/N]"

    while choice not in valid_answers or valid_answers[choice] is None:
        print(color_message(f"{input_message} {default_answer_prompt} ", color), end='')
        choice = input().strip().lower()

    return valid_answers[choice]
