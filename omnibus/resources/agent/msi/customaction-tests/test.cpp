#include "stdafx.h"

TEST(CanInstallTest, ShouldFailIfServiceExistsAndNoUser) {
    CustomActionData customActionCtx;
    bool shouldResetPass;

    bool result = canInstall(true, false, true, customActionCtx, shouldResetPass);

    EXPECT_FALSE(result);
    EXPECT_FALSE(shouldResetPass);
}
